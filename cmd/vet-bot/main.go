package main

import (
	"context"
	"log"
	"os"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

const findingsOwner = "kalexmills"
const findingsRepo = "rangeloop-test-repo"

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
	for {
		err := sampler.Sample(func(r Repository) error {
			VetRepositoryBulk(vetBot, r)
			return nil
		})
		if err != nil {
			break
		}
	}

	// TODO: uniformly sample from some source of repositories and vet them one at a time.
	/*ghToken, ok := os.LookupEnv("GITHUB_TOKEN")

	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	vetBot := NewVetBot(ghToken)

	VetRepositoryBulk(vetBot, Repository{"docker", "engine"})*/
}

// VetBot wraps the GitHub client and context used for all GitHub API requests.
type VetBot struct {
	ctx    context.Context
	client *github.Client
}

// NewVetBot creates a new bot using the provided GitHub token for access.
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
