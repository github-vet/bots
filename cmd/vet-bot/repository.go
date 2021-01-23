package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/asynccapture"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/facts"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/looppointer"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/nestedcallsite"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/ptrcmp"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/writesinput"
	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"github.com/google/go-github/v32/github"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/loader"
)

// Repository encapsulates the information needed to lookup a GitHub repository.
type Repository struct {
	Owner string
	Repo  string
}

// VetResult reports a few lines of code in a GitHub repository for consideration.
type VetResult struct {
	Repository
	FilePath     string
	RootCommitID string
	Quote        string
	Start        token.Position
	End          token.Position
	Message      string
	ExtraInfo    string
}

// Permalink returns the GitHub permalink which refers to the snippet of code retrieved by the VetResult.
func (vr VetResult) Permalink() string {
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", vr.Owner, vr.Repo, vr.RootCommitID, urlEscapeSpaces(vr.Start.Filename), vr.Start.Line, vr.End.Line)
}

func urlEscapeSpaces(str string) string {
	return strings.ReplaceAll(str, " ", "%20")
}

var analyzersToRun = []*analysis.Analyzer{
	inspect.Analyzer,
	ptrcmp.Analyzer,
	writesinput.Analyzer,
	asynccapture.Analyzer,
	nestedcallsite.Analyzer,
	typegraph.Analyzer,
	facts.InductionAnalyzer,
	looppointer.Analyzer,
}

// VetRepositoryBulk streams the contents of a Github repository as a tarball, analyzes each go file, and reports the results.
func VetRepositoryBulk(bot *VetBot, ir *IssueReporter, repo Repository) error {
	rootCommitID, err := GetRootCommitID(bot, repo)
	if err != nil {
		log.Printf("failed to retrieve root commit ID for repo %s/%s", repo.Owner, repo.Repo)
		return err
	}
	url, _, err := bot.client.GetArchiveLink(repo.Owner, repo.Repo, github.Tarball, nil, false)
	if err != nil {
		log.Printf("failed to get tar link for %s/%s: %v", repo.Owner, repo.Repo, err)
		return err
	}
	fset := token.NewFileSet()
	filesByPath, err := getAndUntar(url, repo, fset)
	if err != nil {
		return err
	}
	VetRepo(fset, filesByPath, ReportFinding(ir, fset, rootCommitID, repo))
	stats.FlushStats(bot.statsWriter, repo.Owner, repo.Repo)
	return nil
}

func getAndUntar(url *url.URL, repo Repository, fset *token.FileSet) (map[string][]*ast.File, error) {
	resp, err := http.Get(url.String())
	if err != nil {
		log.Printf("failed to download tar contents: %v", err)
		return nil, err
	}
	defer resp.Body.Close()
	unzipped, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Printf("unable to initialize unzip stream: %v", err)
		return nil, err
	}
	reader := tar.NewReader(unzipped)
	log.Printf("reading contents of %s/%s", repo.Owner, repo.Repo)

	filesByPath := make(map[string][]*ast.File)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("failed to read tar entry")
			return nil, err
		}
		name := header.Name
		split := strings.SplitN(name, "/", 2)
		if len(split) < 2 {
			continue // we only care about files in a subdirectory (due to how GitHub returns archives).
		}
		realName := split[1]
		switch header.Typeflag {
		case tar.TypeReg:
			if IgnoreFile(realName) {
				continue
			}
			bytes, err := ioutil.ReadAll(reader)
			if err != nil {
				log.Printf("error reading contents of %s: %v", realName, err)
				return nil, err
			}
			countLines(realName, bytes)
			stats.AddFile(realName)
			if !IgnoreFile(realName) {
				file, err := parser.ParseFile(fset, realName, bytes, parser.DeclarationErrors)
				if err != nil {
					log.Printf("error parsing file %s: %v", realName, err)
					return nil, err
				}
				filesByPath[path.Dir(realName)] = append(filesByPath[path.Dir(realName)], file)
			}
		}
	}
	return filesByPath, nil
}

func countLines(filename string, contents []byte) {
	lines := bytes.Count(contents, []byte{'\n'})
	stats.AddCount(stats.StatSloc, lines)
	if strings.HasSuffix(filename, "_test.go") {
		stats.AddCount(stats.StatSlocTest, lines)
	}
	if strings.HasPrefix(filename, "vendor") {
		stats.AddCount(stats.StatSlocVendored, lines)
	}
}

// IgnoreFile returns true if the file should be ignored.
func IgnoreFile(filename string) bool {
	if strings.HasPrefix(filename, "vendor/") || strings.HasSuffix(filename, "_test.go") {
		return true
	}
	return !strings.HasSuffix(filename, ".go")
}

// Reporter provides a means to yield a diagnostic function suitable for use by the analysis package which
// also has access to the contents and name of the file being observed.
type Reporter func(map[string][]byte) func(analysis.Diagnostic) // yay for currying!

// VetRepo runs all static analyzers on the parsed set of files provided. When an issue is found,
// the Reporter provided in onFind is triggered.
func VetRepo(fset *token.FileSet, filesByPath map[string][]*ast.File, onFind Reporter) {
	log.Println("vetting repo contents")
	stats.Clear()

	var hardTypeCheckErrs []error
	config := loader.Config{
		Fset:        fset,
		AllowErrors: true,
		TypeChecker: types.Config{
			Importer: importer.ForCompiler(fset, "gc", nil),
			Error: func(err error) {
				if err, ok := err.(types.Error); ok {
					fset.File(err.Pos)
					if !err.Soft {
						hardTypeCheckErrs = append(hardTypeCheckErrs, err)
					}
				}
			},
		},
	}

	packagesToAnalyze := make(map[string]struct{})
	for dir, files := range filesByPath {
		config.CreateFromFiles(dir, files...)
		for _, file := range files {
			// only analyze packages from .go files we found and parsed in the repository
			// needed because config.Load() returns a Program containing standard-library packages
			// we don't care about.
			packagesToAnalyze[file.Name.Name] = struct{}{}
		}
	}

	prog, err := config.Load()

	if err != nil {
		log.Printf("error loading packages: %v", err)
		return
	}
	if hardTypeCheckErrs != nil {
		log.Printf("encountered hard type-checking errors: %v", hardTypeCheckErrs)
		return
	}
	fmt.Printf("type-checked %d packages\n", len(prog.AllPackages))

	for _, pkgInfo := range prog.AllPackages {
		pkgName := pkgInfo.Pkg.Name()
		if _, ok := packagesToAnalyze[pkgName]; !ok {
			continue
		}
		if strings.HasSuffix(pkgName, "_test") {
			continue
		}

		pass := analysis.Pass{
			Fset:  fset,
			Files: pkgInfo.Files,
			Report: func(d analysis.Diagnostic) {
				fmt.Println(d.Message)
			},
			ResultOf:  make(map[*analysis.Analyzer]interface{}),
			TypesInfo: &pkgInfo.Info,
		}

		resetFactBase := util.AddFactBase(&pass)

		for _, analyzer := range analyzersToRun {
			var err error
			pass.ResultOf[analyzer], err = analyzer.Run(&pass)
			if err != nil {
				log.Printf("failed %s analysis: %v", analyzer.Name, err)
				return
			}
			resetFactBase()
		}
	}
	return
}

// GetRootCommitID retrieves the root commit of the default branch of a repository.
func GetRootCommitID(bot *VetBot, repo Repository) (string, error) {
	r, _, err := bot.client.GetRepository(repo.Owner, repo.Repo)
	if err != nil {
		log.Printf("failed to get repo: %v", err)
		return "", err
	}
	defaultBranch := r.GetDefaultBranch()

	// retrieve the root commit of the default branch for the repository
	branch, _, err := bot.client.GetRepositoryBranch(repo.Owner, repo.Repo, defaultBranch)
	if err != nil {
		log.Printf("failed to get default branch: %v", err)
		return "", err
	}
	return branch.GetCommit().GetSHA(), nil
}

// ReportFinding curries several parameters into a function whose signature matches that expected
// by the analysis package for a Diagnostic function.
func ReportFinding(ir *IssueReporter, fset *token.FileSet, rootCommitID string, repo Repository) Reporter {
	return func(contents map[string][]byte) func(analysis.Diagnostic) {
		return func(d analysis.Diagnostic) {
			if len(d.Related) < 1 {
				log.Printf("could not read diagnostic with empty 'Related' field: %v", d.Related)
				return
			}
			filename := d.Related[0].Message
			var extraInfo string
			if len(d.Related) >= 2 {
				extraInfo = d.Related[1].Message
			}
			start := fset.Position(d.Pos)
			end := fset.Position(d.End)
			// split off into a separate thread so any API call to create the issue doesn't block the remaining analysis.
			ir.ReportVetResult(VetResult{
				Repository:   repo,
				FilePath:     fset.File(d.Pos).Name(),
				RootCommitID: rootCommitID,
				Quote:        QuoteFinding(contents[filename], start.Line, end.Line),
				Start:        start,
				End:          end,
				Message:      d.Message,
				ExtraInfo:    extraInfo,
			})
		}
	}
}

// QuoteFinding retrieves the snippet of code that caused the VetResult.
func QuoteFinding(contents []byte, lineStart, lineEnd int) string {
	sc := bufio.NewScanner(bytes.NewReader(contents))
	line := 0
	var sb strings.Builder
	for sc.Scan() && line < lineEnd {
		line++
		if lineStart == line { // truncate whitespace from the first line (fixes formatting later)
			sb.WriteString(strings.TrimSpace(sc.Text()) + "\n")
		}
		if lineStart < line && line <= lineEnd {
			sb.WriteString(sc.Text() + "\n")
		}
	}

	// run go fmt on the snippet to remove leading whitespace
	snippet := sb.String()
	formatted, err := format.Source([]byte(snippet))
	if err != nil {
		return snippet
	}
	return string(formatted)
}
