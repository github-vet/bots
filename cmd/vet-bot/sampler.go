package main

import (
	"encoding/csv"
	"errors"
	"io"
	"log"
	"math/rand"
	"os"
	"sync"
)

// RepositorySampler maintains the state of unvisited repositories and provides a mechanism
// for visiting them at random.
type RepositorySampler struct {
	m           sync.Mutex
	unvisited   []Repository
	visitedFile *MutexWriter
	csvWriter   *csv.Writer
}

// NewRepositorySampler initializes the repository sampler by opening two CSV files. The first file
// consists of the list of repositories from which to sample. The second file -- which will be created
// if it doesn't already exist -- stores a sublist of the repositories which have already been visited.
func NewRepositorySampler(allFile string, visitedFile string) (*RepositorySampler, error) {
	repos, err := readRepositoryList(allFile)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(visitedFile); err == nil {
		visitedRepos, err := readRepositoryList(visitedFile)
		if err != nil {
			return nil, err
		}
		for repo := range visitedRepos {
			delete(repos, repo)
		}
	}
	repoList := make([]Repository, 0, len(repos))
	for repo := range repos {
		repoList = append(repoList, repo)
	}

	visitedWriter, err := os.OpenFile(visitedFile, os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	mw := NewMutexWriter(visitedWriter)
	return &RepositorySampler{
		unvisited:   repoList,
		visitedFile: &mw,
		csvWriter:   csv.NewWriter(&mw),
	}, nil
}

// Sample is used to sample a repository from the list of repositories managed by this sampler. A handler function
// is passed which receives the repository sampled from the list. If the handler returns nil, the sampled repository
// is removed from the list and is not visited again. If the handler returns an error, the sampled repository is
// not removed from the list and may be visited again.
func (gs *RepositorySampler) Sample(handler func(Repository) error) error {
	if len(gs.unvisited) == 0 {
		return errors.New("no unvisited repositories left to sample")
	}
	repo := gs.sampleAndReturn()
	err := handler(repo)

	if err != nil {
		gs.m.Lock()
		defer gs.m.Unlock()
		gs.unvisited = append(gs.unvisited, repo)
		return err
	}

	err = gs.csvWriter.Write([]string{repo.Owner, repo.Repo})
	gs.csvWriter.Flush()
	if err != nil {
		return err
	}
	return nil
}

func (gs *RepositorySampler) sampleAndReturn() Repository {
	gs.m.Lock()
	defer gs.m.Unlock()
	idx := rand.Intn(len(gs.unvisited))
	repo := gs.unvisited[idx]
	gs.unvisited = append(gs.unvisited[:idx], gs.unvisited[idx+1:]...)
	return repo
}

// Close closes the file.
func (gs *RepositorySampler) Close() error {
	return gs.visitedFile.Close()
}

// readRepositoryList retrieves a set of repositories which have already been read from.
func readRepositoryList(filename string) (map[Repository]struct{}, error) {
	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	result := make(map[Repository]struct{})
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) != 2 {
			log.Printf("malformed line in repository list %s", filename)
			continue
		}
		result[Repository{
			Owner: record[0],
			Repo:  record[1],
		}] = struct{}{}
	}
	return result, nil
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
