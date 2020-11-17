package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"github.com/kalexmills/github-vet/cmd/vet-bot/loopclosure"
	"golang.org/x/oauth2"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

const FindingsOwner = "kalexmills"
const FindingsRepo = "rangeloop-test-repo"

func main() {
	ghToken, ok := os.LookupEnv("GITHUB_TOKEN")

	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	vetBot := NewVetBot(ghToken)

	VetRepository(vetBot, Repository{"docker", "engine"})
}

type Repository struct {
	Owner string
	Repo  string
}

type VetBot struct {
	ctx    context.Context
	client *github.Client
}

func NewVetBot(token string) *VetBot {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(token)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return &VetBot{
		ctx:    ctx,
		client: client,
	}
}

type VetResult struct {
	Repository
	RootCommitID string
	Start        token.Position
	End          token.Position
}

func VetRepository(bot *VetBot, repo Repository) {
	// TODO: handle rate limits (sad)... wrap up the GH client inside something a bit smarter.

	// retrieve the default branch for the repository (which may not be 'master' or 'main')
	r, _, err := bot.client.Repositories.Get(bot.ctx, repo.Owner, repo.Repo)
	if err != nil {
		log.Printf("failed to get repo: %v", err)
		return
	}
	defaultBranch := r.GetDefaultBranch()

	// retrieve the root commit of the default branch for the repository
	branch, _, err := bot.client.Repositories.GetBranch(bot.ctx, repo.Owner, repo.Repo, defaultBranch)
	if err != nil {
		log.Printf("failed to get default branch: %v", err)
		return
	}
	rootSha := branch.GetCommit().GetCommit().GetTree().GetSHA()
	rootCommitID := branch.GetCommit().GetSHA()

	// get a recursive list of all files found under the root commit.
	tree, _, err := bot.client.Git.GetTree(bot.ctx, repo.Owner, repo.Repo, rootSha, true)
	if err != nil {
		log.Printf("failed to get tree: %v", err)
		return
	}

	fset := token.NewFileSet()
	var files []*ast.File

	// retrieve and parse the repository contents one file at a time.
	for _, content := range tree.Entries {
		if strings.HasSuffix(content.GetPath(), ".pb.go") {
			continue // special case to ignore
		}
		if strings.HasSuffix(content.GetPath(), ".go") {
			log.Printf("retrieving file: %s", content.GetPath())

			bytes, _, err := bot.client.Git.GetBlobRaw(bot.ctx, repo.Owner, repo.Repo, content.GetSHA())
			if err != nil {
				log.Printf("fatal error getting file %s", content.GetPath())
				continue
			}

			file, err := parser.ParseFile(fset, content.GetPath(), string(bytes), parser.AllErrors)
			if err != nil {
				log.Printf("error when parsing %s: %v\n", content.GetPath(), err)
				continue
			}
			files = append(files, file)
		}
	}

	// TODO: since we can do the analysis without type-checking, we don't need to read the entire repo before we even start (anymore).
	//       move this analysis into the above loop so we don't have to keep around an ever-growing FileSet and can run in an even smaller memory footprint.

	// run a static code analysis across the entire repo
	pass := analysis.Pass{
		Fset:  fset,
		Files: files,
		Report: func(d analysis.Diagnostic) {
			HandleVetResult(bot, VetResult{
				Repository:   repo,
				RootCommitID: rootCommitID,
				Start:        fset.Position(d.Pos),
				End:          fset.Position(d.End),
			})
		},
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

func HandleVetResult(bot *VetBot, result VetResult) {
	// TODO: record this finding in structured form some place.
	iss, _, err := bot.client.Issues.Create(bot.ctx, FindingsOwner, FindingsRepo, CreateIssueRequest(result))
	if err != nil {
		log.Printf("error opening new issue: %v", err)
		return
	}
	log.Printf("opened new issue at %s", iss.GetURL())
}

func CreateIssueRequest(result VetResult) *github.IssueRequest {
	title := fmt.Sprintf("%s/%s: ", result.Owner, result.Repo)
	permalink := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", result.Owner, result.Repo, result.RootCommitID, result.Start.Filename, result.Start.Line, result.End.Line)

	// TODO: make the issue prettier; include a snippet of the source and link to it
	// Also provide some context as to why the bot thinks the code is wrong.
	body := "Found an issue at " + permalink
	return &github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
}
