package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/kalexmills/github-vet/internal/ratelimit"
	"golang.org/x/oauth2"

	"net/http"
	_ "net/http/pprof"
)

const findingsOwner = "kalexmills"         // "github-vet"
const findingsRepo = "rangeloop-test-repo" //"rangeclosure-findings"

// main runs the vetbot.
//
// vetbot runs continuously, sampling from a list of GitHub repositories, downloading their contents, running
// static analysis on every .go file they contain, and reporting any findings to the issue tracker of a hardcoded
// GitHub repository.
//
// vetbot expects an environment variable named GITHUB_TOKEN which contains a valid personal access token used
// to authenticate with the GitHub API.
//
// vetbot expects read-write access to the working directory. vetbot expects a non-empty file named 'repos.csv',
// which contains a list of GitHub repositories to sample from. This file should contain 'owner,repo' pairs, one per
// line.
//
// vetbot creates two other files, 'visited.csv' and 'issues.csv' to track issues opened and the repositories which
// have been visted.
//
// vetbot also creates a log file named 'MM-DD-YYYY.log', using the system date.
func main() {
	flag.Parse()

	logFilename := time.Now().Format("01-02-2006") + ".log"
	logFile, err := os.OpenFile(logFilename, os.O_APPEND|os.O_CREATE, 0666)
	defer logFile.Close()
	if err != nil {
		log.Fatalf("cannot open log file for writing: %v", err)
	}
	log.SetOutput(logFile)

	sampler, err := NewRepositorySampler("repos.csv", "visited.csv")
	defer sampler.Close()
	if err != nil {
		log.Fatalf("can't start sampler: %v", err)
	}

	ghToken, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	vetBot := NewVetBot(ghToken)
	issueReporter, err := NewIssueReporter(&vetBot, "issues.csv")
	if err != nil {
		log.Fatalf("can't start issue reporter: %v", err)
	}

	go sampleRepos(&vetBot, sampler, issueReporter)

	http.ListenAndServe("localhost:8080", nil)
}

func sampleRepos(vetBot *VetBot, sampler *RepositorySampler, issueReporter *IssueReporter) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	for {
		select {
		case <-interrupt:
			vetBot.wg.Wait()
			return
		default:
			err := sampler.Sample(func(r Repository) error {
				return VetRepositoryBulk(vetBot, issueReporter, r)
			})
			if err != nil {
				log.Printf("stopping scan due to error :%v", err)
				break
			}
		}
	}
}

// VetBot wraps the GitHub client and context used for all GitHub API requests.
type VetBot struct {
	client     *ratelimit.Client
	reportFunc func(bot *VetBot, result VetResult)
	wg         sync.WaitGroup
}

// NewVetBot creates a new bot using the provided GitHub token for access.
func NewVetBot(token string) VetBot {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(token)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	limited, err := ratelimit.NewClient(ctx, client)
	if err != nil {
		panic(err)
	}
	return VetBot{
		client: &limited,
	}
}
