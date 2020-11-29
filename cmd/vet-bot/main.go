package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v32/github"
	"github.com/kalexmills/github-vet/internal/ratelimit"
	"golang.org/x/oauth2"

	_ "net/http/pprof"
)

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
	opts := parseOpts()

	logFilename := time.Now().Format("01-02-2006") + ".log"
	logFile, err := os.OpenFile(logFilename, os.O_APPEND|os.O_CREATE, 0666)
	defer logFile.Close()
	if err != nil {
		log.Fatalf("cannot open log file for writing: %v", err)
	}
	log.SetOutput(logFile)

	sampler, err := NewRepositorySampler(opts.ReposFile, opts.VisitedFile)
	defer sampler.Close()
	if err != nil {
		log.Fatalf("can't start sampler: %v", err)
	}
	vetBot := NewVetBot(opts.GithubToken)
	issueReporter, err := NewIssueReporter(&vetBot, opts.IssuesFile, opts.Owner, opts.Repo)
	if err != nil {
		log.Fatalf("can't start issue reporter: %v", err)
	}

	sampleRepos(&vetBot, sampler, issueReporter)
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
			debug.FreeOSMemory()
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

type opts struct {
	GithubToken string
	IssuesFile  string
	ReposFile   string
	VisitedFile string
	Owner       string
	Repo        string
}

var defaultOpts opts = opts{
	IssuesFile:  "issues.csv",
	ReposFile:   "repos.csv",
	VisitedFile: "visited.csv",
	Owner:       "github-vet",
	Repo:        "rangeclosure-findings",
}

func parseOpts() opts {
	var ok bool
	result := defaultOpts
	result.GithubToken, ok = os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatalf("could not find GITHUB_TOKEN environment variable")
	}
	flag.StringVar(&result.IssuesFile, "issues", result.IssuesFile, "path to issues CSV file")
	flag.StringVar(&result.ReposFile, "repos", result.ReposFile, "path to repos CSV file")
	flag.StringVar(&result.VisitedFile, "visited", result.VisitedFile, "path to visited CSV file")
	ownerStr := flag.String("repo", result.Owner+"/"+result.Repo, "owner/repository of GitHub repo where issues will be filed")
	flag.Parse()

	repoToks := strings.Split(*ownerStr, "/")
	if len(repoToks) != 2 {
		log.Fatalf("could not parse repo flag '%s' which must be in owner/repository format", *ownerStr)
	}
	result.Owner = repoToks[0]
	result.Repo = repoToks[1]

	return result
}
