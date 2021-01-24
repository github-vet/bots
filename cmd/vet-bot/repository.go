package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/github-vet/bots/cmd/vet-bot/analysis/asynccapture"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/facts"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/looppointer"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/nestedcallsite"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/packid"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/ptrcmp"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/typegraph"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/util"
	"github.com/github-vet/bots/cmd/vet-bot/analysis/writesinput"
	"github.com/github-vet/bots/cmd/vet-bot/stats"
	"github.com/google/go-github/v32/github"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/packages"
)

// Repository encapsulates the information needed to lookup a GitHub repository.
type Repository struct {
	Owner string
	Repo  string
}

func (r Repository) String() string {
	return r.Owner + "/" + r.Repo
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
	packid.Analyzer,
	looppointer.Analyzer,
}

func VetRepositoryToDisk(bot *VetBot, ir *IssueReporter, repo Repository) (err error) {
	/*rootCommitID, err := GetRootCommitID(bot, repo)
	if err != nil {
		log.Printf("failed to retrieve root commit ID for repo %s/%s", repo.Owner, repo.Repo)
		return err
	}*/
	stats.Clear()
	url, _, err := bot.client.GetArchiveLink(repo.Owner, repo.Repo, github.Tarball, nil, false)
	if err != nil {
		log.Printf("failed to get tar link for %s/%s: %v", repo.Owner, repo.Repo, err)
		return err
	}
	err = untarViaHttpToDir(url, repo, bot.opts.UntarDir)
	defer func() {
		err = cleanWorkDir(bot.opts.UntarDir)
	}()
	if err != nil {
		log.Printf("could not untar archive to %s", bot.opts.UntarDir)
		return err
	}

	return VetReposFromDisk(bot, ir, repo, bot.opts.UntarDir)
}

func untarViaHttpToDir(url *url.URL, repo Repository, outDir string) error {
	resp, err := http.Get(url.String())
	if err != nil {
		log.Printf("failed to download tar contents: %v", err)
		return err
	}
	defer resp.Body.Close()
	return untar(resp.Body, outDir, repo)
}

func VetReposFromDisk(bot *VetBot, ir *IssueReporter, repo Repository, workDir string) error {
	srcDir, err := getRepoDirectory(workDir)
	if err != nil {
		return err
	}
	var fset token.FileSet
	config := packages.Config{
		Dir:  srcDir,
		Fset: &fset,
		Mode: packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedTypes,
	}
	pkgs, err := packages.Load(&config, "./...")
	if err != nil {
		log.Printf("error loading packages: %v", err)
		return err
	}
	log.Printf("type-checked %d packages\n", len(pkgs))

	for _, pkg := range pkgs {

		pass := analysis.Pass{
			Fset:  pkg.Fset,
			Files: pkg.Syntax,
			Report: func(d analysis.Diagnostic) {
				fmt.Println(d.Message, d.Related)
			},
			ResultOf:  make(map[*analysis.Analyzer]interface{}),
			TypesInfo: pkg.TypesInfo,
			Pkg:       pkg.Types,
		}

		resetFactBase := util.AddFactBase(&pass)

		for _, analyzer := range analyzersToRun {
			var err error
			pass.ResultOf[analyzer], err = analyzer.Run(&pass)
			if err != nil {
				log.Printf("failed %s analysis: %v", analyzer.Name, err)
				return err
			}
			resetFactBase()
		}
	}

	stats.FlushStats(bot.statsWriter, repo.Owner, repo.Repo)
	return nil
}

// gitRepoDirectory returns the first directory found as a subdirectory of the provided
// workDir.
func getRepoDirectory(workDir string) (string, error) {
	files, err := ioutil.ReadDir(workDir)
	if err != nil {
		return "", err
	}
	for _, finfo := range files {
		if finfo.IsDir() {
			return path.Join(workDir, finfo.Name()), nil
		}
	}
	return "", errors.New("expected GitHub tarball to contain a directory")
}

func cleanWorkDir(workDir string) error {
	files, err := ioutil.ReadDir(workDir)
	if err != nil {
		return err
	}
	for _, finfo := range files {
		err := os.RemoveAll(path.Join(workDir, finfo.Name()))
		if err != nil {
			return err
		}
	}
	return nil
}

type Reporter func(map[string][]byte) func(analysis.Diagnostic) // yay for currying!

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
