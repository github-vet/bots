package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"

	"github.com/github-vet/bots/cmd/vet-bot/acceptlist"
	"github.com/github-vet/bots/internal/db"
	"github.com/github-vet/bots/internal/ratelimit"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"

	_ "github.com/mattn/go-sqlite3"
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
	opts, err := parseOpts()
	if err != nil {
		log.Fatalf("error during config: %v", err)
	}
	log.Printf("configured options: %+v", opts)

	vetBot := NewVetBot(opts.GithubToken, opts)
	defer vetBot.Close()

	issueReporter, err := NewIssueReporter(&vetBot, opts.TargetOwner, opts.TargetRepo)

	if err != nil {
		log.Fatalf("can't start issue reporter: %v", err)
	}

	if opts.AcceptListPath != "" {
		err := acceptlist.LoadAcceptList(opts.AcceptListPath)
		if err != nil {
			log.Fatalf("cannot read accept list: %v", err)
		}
	}

	if opts.SingleRepo == "" {
		sampler, err := NewRepositorySampler(vetBot.db)
		if err != nil {
			log.Fatalf("can't start sampler: %v", err)
		}
		sampleRepos(&vetBot, sampler, issueReporter)
	} else {
		sampleRepo(&vetBot, issueReporter)
	}
}

func sampleRepos(vetBot *VetBot, sampler *RepositorySampler, issueReporter *IssueReporter) {
	log.Println("entering repository sampling loop")
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
				log.Printf("stopping scan due to error: %v", err)
				return
			}
			// the following line was found to reduce memory usage on Windows; it may not be
			// necessary on all OS's
			debug.FreeOSMemory()
		}
	}
}

func sampleRepo(vetBot *VetBot, issueReporter *IssueReporter) {
	err := VetRepositoryBulk(vetBot, issueReporter, Repository{
		Owner: vetBot.opts.SingleOwner,
		Repo:  vetBot.opts.SingleRepo,
	})
	if err != nil {
		log.Printf("error: %v", err)
	}
	vetBot.wg.Wait()
}

// VetBot wraps the GitHub client and context used for all GitHub API requests.
type VetBot struct {
	client      *ratelimit.Client
	wg          sync.WaitGroup
	opts        opts
	db          *sql.DB
	statsFile   *MutexWriter
	statsWriter *csv.Writer
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

	DB, err := sql.Open("sqlite3", opts.DatabaseFile)
	if err != nil {
		log.Fatalf("cannot open database from %s: %v", opts.DatabaseFile, err)
	}

	// bootstrap the schema
	if opts.DbBootstrapFolder != "" {
		err := db.BootstrapDB(opts.DbBootstrapFolder, DB)
		if err != nil {
			log.Fatalf("could not bootstrap database: %v", err)
		}
	}

	// seed the initial set of repositories
	if opts.ReposFile != "" {
		err := db.SeedRepositories(opts.ReposFile, DB)
		if err != nil {
			log.Fatalf("could not seed database with initial repositories: %v", err)
		}
	}

	statsFile, err := os.OpenFile(opts.StatsFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("cannot open stats file from %s: %v", opts.StatsFile, err)
	}
	mw := NewMutexWriter(statsFile)
	return VetBot{
		client:      &limited,
		db:          DB,
		opts:        opts,
		statsFile:   &mw,
		statsWriter: csv.NewWriter(&mw),
	}
}

// Close closes any open files and database connections.
func (vb *VetBot) Close() {
	vb.statsFile.Close()
	vb.db.Close()
}

// MutexWriter wraps an io.WriteCloser with a sync.Mutex.
type MutexWriter struct { // TODO: this might be a bit much; we already have a Mutex in RepositorySampler
	m sync.Mutex
	w io.WriteCloser
}

// NewMutexWriter wraps an io.WriteCloser with a sync.Mutex.
func NewMutexWriter(w io.WriteCloser) MutexWriter {
	return MutexWriter{w: w}
}

func (mw *MutexWriter) Write(b []byte) (int, error) {
	mw.m.Lock()
	defer mw.m.Unlock()
	return mw.w.Write(b)
}

// Close closes the underlying WriteCloser.
func (mw *MutexWriter) Close() error {
	mw.m.Lock()
	defer mw.m.Unlock()
	return mw.w.Close()
}
