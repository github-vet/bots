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

	"github.com/google/go-github/github"
	"github.com/kalexmills/github-vet/cmd/vet-bot/loopclosure"
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
}

// Permalink returns the GitHub permalink which refers to the snippet of code retrieved by the VetResult.
func (vr VetResult) Permalink() string {
	fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", vr.Owner, vr.Repo, vr.RootCommitID, vr.Start.Filename, vr.Start.Line, vr.End.Line)
}

// VetRepositoryBulk streams the contents of a Github repository as a tarball, analyzes each go file, and reports the results.
func VetRepositoryBulk(bot *VetBot, ir *IssueReporter, repo Repository) {
	rootCommitID, err := GetRootCommitID(bot, repo)
	if err != nil {
		log.Printf("failed to retrieve root commit ID for repo %s/%s", repo.Owner, repo.Repo)
		return
	}
	url, _, err := bot.client.Repositories.GetArchiveLink(bot.ctx, repo.Owner, repo.Repo, github.Tarball, nil)
	if err != nil {
		log.Printf("failed to get tar link for %s/%s: %v", repo.Owner, repo.Repo, err)
		return
	}
	fmt.Println(url.String())
	resp, err := http.Get(url.String())
	if err != nil {
		log.Printf("failed to download tar contents: %v", err)
		return
	}
	defer resp.Body.Close()
	unzipped, err := gzip.NewReader(resp.Body)
	if err != nil {
		log.Printf("unable to initialize unzip stream: %v", err)
		return
	}
	reader := tar.NewReader(unzipped)
	fset := token.NewFileSet()
	reporter := ReportFinding(ir, fset, rootCommitID, repo)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("failed to read tar entry")
			break
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
			log.Printf("found interesting file %s", realName)
			bytes, err := ioutil.ReadAll(reader)
			if err != nil {
				log.Printf("error reading contents of %s: %v", realName, err)
			}
			VetFile(bytes, realName, fset, reporter)
		}
	}
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
type Reporter func(string, []byte) func(analysis.Diagnostic) // yay for currying!

// VetFile parses and runs static analyis on the file contents it is passed.
func VetFile(contents []byte, path string, fset *token.FileSet, onFind Reporter) {
	file, err := parser.ParseFile(fset, path, string(contents), parser.AllErrors)
	if err != nil {
		log.Printf("failed to parse file %s: %v", path, err)
	}
	pass := analysis.Pass{
		Fset:     fset,
		Files:    []*ast.File{file},
		Report:   onFind(path, contents),
		ResultOf: make(map[*analysis.Analyzer]interface{}),
	}
	inspection, err := inspect.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed inspection: %v", err)
	}
	pass.ResultOf[inspect.Analyzer] = inspection
	_, err = loopclosure.Analyzer.Run(&pass)
	if err != nil {
		log.Printf("failed analysis: %v", err)
	}
}

// GetRootCommitID retrieves the root commit of the default branch of a repository.
func GetRootCommitID(bot *VetBot, repo Repository) (string, error) {
	r, _, err := bot.client.Repositories.Get(bot.ctx, repo.Owner, repo.Repo)
	if err != nil {
		log.Printf("failed to get repo: %v", err)
		return "", err
	}
	defaultBranch := r.GetDefaultBranch()

	// retrieve the root commit of the default branch for the repository
	branch, _, err := bot.client.Repositories.GetBranch(bot.ctx, repo.Owner, repo.Repo, defaultBranch)
	if err != nil {
		log.Printf("failed to get default branch: %v", err)
		return "", err
	}
	return branch.GetCommit().GetSHA(), nil
}

// ReportFinding curries several parameters into an appopriate Diagnostic report function.
func ReportFinding(ir *IssueReporter, fset *token.FileSet, rootCommitID string, repo Repository) Reporter {
	return func(path string, contents []byte) func(analysis.Diagnostic) {
		return func(d analysis.Diagnostic) {
			// split off into a separate thread so any API call to create the issue doesn't block the remaining analysis.

			ir.ReportVetResult(VetResult{
				Repository:   repo,
				FilePath:     path,
				RootCommitID: rootCommitID,
				FileContents: contents,
				Start:        fset.Position(d.Pos),
				End:          fset.Position(d.End),
			})
		}
	}
}
