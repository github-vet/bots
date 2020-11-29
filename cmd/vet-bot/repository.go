package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/kalexmills/github-vet/cmd/vet-bot/callgraph"
	"github.com/kalexmills/github-vet/cmd/vet-bot/loopclosure"
	"github.com/kalexmills/github-vet/cmd/vet-bot/looppointer"
	"github.com/kalexmills/github-vet/cmd/vet-bot/nogofunc"
	"github.com/kalexmills/github-vet/cmd/vet-bot/pointerescapes"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
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
	FileContents []byte
	Start        token.Position
	End          token.Position
	Message      string
}

// Permalink returns the GitHub permalink which refers to the snippet of code retrieved by the VetResult.
func (vr VetResult) Permalink() string {
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", vr.Owner, vr.Repo, vr.RootCommitID, vr.Start.Filename, vr.Start.Line, vr.End.Line)
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
	contents := make(map[string][]byte)
	var files []*ast.File
	if err := func() error {
		resp, err := http.Get(url.String())
		if err != nil {
			log.Printf("failed to download tar contents: %v", err)
			return err
		}
		defer resp.Body.Close()
		unzipped, err := gzip.NewReader(resp.Body)
		if err != nil {
			log.Printf("unable to initialize unzip stream: %v", err)
			return err
		}
		reader := tar.NewReader(unzipped)
		log.Printf("reading contents of %s/%s", repo.Owner, repo.Repo)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("failed to read tar entry")
				return err
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
				}
				file, err := parser.ParseFile(fset, realName, string(bytes), parser.AllErrors)
				if err != nil {
					log.Printf("failed to parse file %s: %v", realName, err)
					continue
				}
				files = append(files, file)
				contents[fset.File(file.Pos()).Name()] = bytes
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	VetRepo(contents, files, fset, ReportFinding(ir, fset, rootCommitID, repo))
	return nil
}

// IgnoreFile returns true if the file should be ignored.
func IgnoreFile(filename string) bool {
	if strings.HasSuffix(filename, ".pb.go") {
		return true
	}
	return !strings.HasSuffix(filename, ".go")
}

// Reporter provides a means to yield a diagnostic function suitable for use by the analysis package which
// also has access to the contents and name of the file being observed.
type Reporter func(map[string][]byte) func(analysis.Diagnostic) // yay for currying!

func VetRepo(contents map[string][]byte, files []*ast.File, fset *token.FileSet, onFind Reporter) {
	pass := analysis.Pass{
		Fset:     fset,
		Files:    files,
		Report:   onFind(contents),
		ResultOf: make(map[*analysis.Analyzer]interface{}),
	}
	var err error
	pass.ResultOf[inspect.Analyzer], err = inspect.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed inspection analysis: %v", err)
		return
	}

	pass.ResultOf[callgraph.Analyzer], err = callgraph.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed callgraph analysis: %v", err)
		return
	}

	pass.ResultOf[nogofunc.Analyzer], err = nogofunc.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed nogofunc analysis: %v", err)
		return
	}

	pass.ResultOf[pointerescapes.Analyzer], err = pointerescapes.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed pointerescapes analysis: %v", err)
		return
	}

	_, err = loopclosure.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed loopclosure analysis: %v", err)
	}
	_, err = looppointer.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed looppointer analysis: %v", err)
	}
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

// ReportFinding curries several parameters into an appopriate Diagnostic report function.
func ReportFinding(ir *IssueReporter, fset *token.FileSet, rootCommitID string, repo Repository) Reporter {
	return func(contents map[string][]byte) func(analysis.Diagnostic) {
		return func(d analysis.Diagnostic) {
			if len(d.Related) != 1 {
				log.Printf("could not read diagnostic with unexpected 'Related' field: %v", d.Related)
				return
			}
			filename := d.Related[0].Message
			// split off into a separate thread so any API call to create the issue doesn't block the remaining analysis.
			ir.ReportVetResult(VetResult{
				Repository:   repo,
				FilePath:     filename,
				RootCommitID: rootCommitID,
				FileContents: contents[filename],
				Start:        fset.Position(d.Pos),
				End:          fset.Position(d.End),
				Message:      d.Message,
			})
		}
	}
}
