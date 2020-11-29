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

	vetBot := NewVetBot(opts.GithubToken, opts)
	issueReporter, err := NewIssueReporter(&vetBot, opts.IssuesFile, opts.TargetOwner, opts.TargetRepo)
	if err != nil {
		log.Fatalf("can't start issue reporter: %v", err)
	}

	if opts.SingleRepo == "" {
		sampler, err := NewRepositorySampler(opts.ReposFile, opts.VisitedFile)
		defer sampler.Close()
		if err != nil {
			log.Fatalf("can't start sampler: %v", err)
		}
		sampleRepos(&vetBot, sampler, issueReporter)
	} else {
		sampleRepo(&vetBot, issueReporter)
	}
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

func sampleRepo(vetBot *VetBot, issueReporter *IssueReporter) {
	VetRepositoryBulk(vetBot, issueReporter, Repository{
		Owner: vetBot.opts.SingleOwner,
		Repo:  vetBot.opts.SingleRepo,
	})
}

// VetBot wraps the GitHub client and context used for all GitHub API requests.
type VetBot struct {
	client     *ratelimit.Client
	reportFunc func(bot *VetBot, result VetResult)
	wg         sync.WaitGroup
	opts       opts
}

// NewVetBot creates a new bot using the provided GitHub token for access.
func NewVetBot(token string, opts opts) VetBot {
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
		opts:   opts,
	}
}

type opts struct {
	GithubToken string
	IssuesFile  string
	ReposFile   string
	VisitedFile string
	TargetOwner string
	TargetRepo  string
	SingleOwner string
	SingleRepo  string
}

var defaultOpts opts = opts{
	IssuesFile:  "issues.csv",
	ReposFile:   "repos.csv",
	VisitedFile: "visited.csv",
	TargetOwner: "github-vet",
	TargetRepo:  "rangeclosure-findings",
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
	singleStr := flag.String("single", "", "owner/repository of a single repository to read")
	ownerStr := flag.String("repo", result.TargetOwner+"/"+result.TargetRepo, "owner/repository of GitHub repo where issues will be filed")
	flag.Parse()

	if *singleStr != "" {
		result.SingleOwner, result.SingleRepo = parseRepoString(*singleStr, "single")
	}
	result.TargetOwner, result.TargetRepo = parseRepoString(*ownerStr, "repo")

	return result
}

func parseRepoString(str string, flag string) (string, string) {
	repoToks := strings.Split(str, "/")
	if len(repoToks) != 2 {
		log.Fatalf("could not parse %s flag '%s' which must be in owner/repository format", flag, str)
	}
	return repoToks[0], repoToks[1]
}
