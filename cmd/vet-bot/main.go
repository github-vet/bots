package main

import (
	"context"
	"log"
	"os"
	"sync"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const findingsOwner = "github-vet"
const findingsRepo = "rangeloop-findings"

func main() {
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
	for {
		err := sampler.Sample(func(r Repository) error {
			VetRepositoryBulk(&vetBot, issueReporter, r)
			return nil
		})
		if err != nil {
			break
		}
	}

	vetBot.wg.Wait()
	/*ghToken, ok := os.LookupEnv("GITHUB_TOKEN")

	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	vetBot := NewVetBot(ghToken)
	issueReporter, err := NewIssueReporter(&vetBot, "issues.csv")
	if err != nil {
		panic(err)
	}

	VetRepositoryBulk(&vetBot, issueReporter, Repository{"kalexmills", "bad-go"})
	vetBot.wg.Wait()*/
}

// VetBot wraps the GitHub client and context used for all GitHub API requests.
type VetBot struct {
	ctx        context.Context
	client     *github.Client
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
	return VetBot{
		ctx:    ctx,
		client: client,
	}
}
