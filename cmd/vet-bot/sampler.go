package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"math/rand"
	"sync"

	"github.com/github-vet/bots/internal/db"
)

// RepositorySampler maintains the state of unvisited repositories and provides a mechanism
// for visiting them at random.
type RepositorySampler struct {
	m         sync.Mutex
	unvisited []Repository
	db        *sql.DB
}

// NewRepositorySampler initializes the repository sampler by opening two CSV files. The first file
// consists of the list of repositories from which to sample. The second file -- which will be created
// if it doesn't already exist -- stores a sublist of the repositories which have already been visited.
func NewRepositorySampler(db *sql.DB) (*RepositorySampler, error) {
	repos, err := readFreshRepositories(db)
	if err != nil {
		return nil, err
	}
	return &RepositorySampler{
		unvisited: repos,
		db:        db,
	}, nil
}

// Sample is used to sample a repository from the list of repositories managed by this sampler. A handler function
// is passed which receives the repository sampled from the list. If the handler returns nil, the sampled repository
// is removed from the list and is not visited again. If the handler returns an error, the sampled repository is
// not removed from the list and may be visited again. Sample only returns an error itself if no further samples should
// be made.
func (rs *RepositorySampler) Sample(handler func(Repository) error) error {
	if len(rs.unvisited) == 0 {
		// TODO: double-check the database again and continue... or throw an error and stop
		return errors.New("no unvisited repositories left to sample")
	}

	repo := rs.sampleAndReturn()
	err := handler(repo)

	if err != nil {
		rs.m.Lock()
		defer rs.m.Unlock()
		rs.unvisited = append(rs.unvisited, repo)
		log.Printf("repo %s/%s will be tried again despite error: %v", repo.Owner, repo.Repo, err)
		return nil
	}

	_, err = db.RepositoryDAO.Upsert(context.Background(), rs.db, db.Repository{
		GithubOwner: repo.Owner,
		GithubRepo:  repo.Repo,
		State:       db.RepoStateVisited,
	})
	return err
}

func (rs *RepositorySampler) sampleAndReturn() Repository {
	rs.m.Lock()
	defer rs.m.Unlock()
	idx := rand.Intn(len(rs.unvisited))
	repo := rs.unvisited[idx]
	rs.unvisited[idx] = rs.unvisited[len(rs.unvisited)-1]
	rs.unvisited = rs.unvisited[:len(rs.unvisited)-1]
	return repo
}

func readFreshRepositories(database *sql.DB) ([]Repository, error) {
	freshRepos, err := db.RepositoryDAO.ListByState(context.Background(), database, db.RepoStateFresh)
	if err != nil {
		return nil, err
	}

	result := make([]Repository, 0, len(freshRepos))
	for _, dbRepo := range freshRepos {
		result = append(result, Repository{
			Owner: dbRepo.GithubOwner,
			Repo:  dbRepo.GithubRepo,
		})
	}
	return result, nil
}
