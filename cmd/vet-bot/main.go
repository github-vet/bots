package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"strings"

	"github.com/google/go-github/github"
	"github.com/kalexmills/github-vet/cmd/vet-bot/loopclosure"
	"golang.org/x/oauth2"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

var owner = "kalexmills"
var repo = "bad-go"

func main() {
	// setup the OAuth2 client.
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: "OMITTED"},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Fatalf("failed to get repo: %v", err)
	}
	defaultBranch := r.GetDefaultBranch()

	branch, _, err := client.Repositories.GetBranch(ctx, owner, repo, defaultBranch)
	if err != nil {
		log.Fatalf("failed to get default branch: %v", err)
	}
	rootSha := branch.GetCommit().GetCommit().GetTree().GetSHA()

	fmt.Println(rootSha)
	tree, _, err := client.Git.GetTree(ctx, owner, repo, rootSha, true)
	if err != nil {
		log.Fatalf("failed to get tree: %v", err)
	}

	fset := token.NewFileSet()
	filesByPackage := make(map[string][]*ast.File)

	for _, content := range tree.Entries {
		if strings.HasSuffix(content.GetPath(), ".pb.go") {
			continue
		}
		if strings.HasSuffix(content.GetPath(), ".go") {
			bytes, _, err := client.Git.GetBlobRaw(ctx, owner, repo, content.GetSHA())
			if err != nil {
				log.Fatalf("fatal error getting sha %s", content.GetSHA())
			}

			file, err := parser.ParseFile(fset, content.GetPath(), string(bytes), parser.AllErrors)
			if err != nil {
				continue
			}
			if pkgFiles, ok := filesByPackage[file.Name.String()]; ok {
				pkgFiles = append(pkgFiles, file)
			} else {
				filesByPackage[file.Name.String()] = make([]*ast.File, 1)
				filesByPackage[file.Name.String()][0] = file
			}
		}
	}
	for pkgName, pkgFiles := range filesByPackage {
		log.Printf("checking package %s", pkgName)

		// run analysis
		pass := analysis.Pass{
			Fset:  fset,
			Files: pkgFiles,
			Report: func(d analysis.Diagnostic) {
				fmt.Printf("diagnostic result from %s to %s\n", fset.Position(d.Pos), fset.Position(d.End))
			},
			ResultOf: make(map[*analysis.Analyzer]interface{}),
		}
		inspection, err := inspect.Analyzer.Run(&pass)
		if err != nil {
			log.Fatalf("failed inspection for package %s: %v", pkgName, err)
		}
		pass.ResultOf[inspect.Analyzer] = inspection
		_, err = loopclosure.Analyzer.Run(&pass)
		if err != nil {
			log.Fatalf("failed analysis for package %s: %v", pkgName, err)
		}
	}
}

type Repository struct {
	Owner string
	Repo  string
}

func VetRepository(client *github.Client, repo Repository) {

	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		log.Fatalf("failed to get repo: %v", err)
	}
	defaultBranch := r.GetDefaultBranch()

	branch, _, err := client.Repositories.GetBranch(ctx, owner, repo, defaultBranch)
	if err != nil {
		log.Fatalf("failed to get default branch: %v", err)
	}
	rootSha := branch.GetCommit().GetCommit().GetTree().GetSHA()

	fmt.Println(rootSha)
	tree, _, err := client.Git.GetTree(ctx, owner, repo, rootSha, true)
	if err != nil {
		log.Fatalf("failed to get tree: %v", err)
	}

	fset := token.NewFileSet()
	filesByPackage := make(map[string][]*ast.File)

	for _, content := range tree.Entries {
		if strings.HasSuffix(content.GetPath(), ".pb.go") {
			continue
		}
		if strings.HasSuffix(content.GetPath(), ".go") {
			bytes, _, err := client.Git.GetBlobRaw(ctx, owner, repo, content.GetSHA())
			if err != nil {
				log.Fatalf("fatal error getting sha %s", content.GetSHA())
			}

			file, err := parser.ParseFile(fset, content.GetPath(), string(bytes), parser.AllErrors)
			if err != nil {
				continue
			}
			if pkgFiles, ok := filesByPackage[file.Name.String()]; ok {
				pkgFiles = append(pkgFiles, file)
			} else {
				filesByPackage[file.Name.String()] = make([]*ast.File, 1)
				filesByPackage[file.Name.String()][0] = file
			}
		}
	}
	for pkgName, pkgFiles := range filesByPackage {
		log.Printf("checking package %s", pkgName)

		// run analysis
		pass := analysis.Pass{
			Fset:  fset,
			Files: pkgFiles,
			Report: func(d analysis.Diagnostic) {
				fmt.Printf("diagnostic result from %s to %s\n", fset.Position(d.Pos), fset.Position(d.End))
			},
			ResultOf: make(map[*analysis.Analyzer]interface{}),
		}
		inspection, err := inspect.Analyzer.Run(&pass)
		if err != nil {
			log.Fatalf("failed inspection for package %s: %v", pkgName, err)
		}
		pass.ResultOf[inspect.Analyzer] = inspection
		_, err = loopclosure.Analyzer.Run(&pass)
		if err != nil {
			log.Fatalf("failed analysis for package %s: %v", pkgName, err)
		}
	}
}
