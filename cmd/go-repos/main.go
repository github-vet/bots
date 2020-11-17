package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// 699280 Golang repositories are on GitHub

const pageSize = 100
const expectedClockSkewMillis = 10

// go-repos is a slow process which uses the Search API to find a bunch of unique Golang
// repos using a cheap trick. A better trick is now known.
func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: go run main.go [start-key] [outfile]")
		os.Exit(1)
	}

	if len(os.Args[1]) != 2 {
		fmt.Println("usage: pass a 2-character string as the start-key to start from")
		os.Exit(1)
	}

	startKey := os.Args[1]
	filename := os.Args[2]

	// setup the OAuth2 client.
	ctx := context.Background()
	token, err := ioutil.ReadFile("token.txt")
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(token)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// find out how many Golang repositories Github has in total; to get a sense of how representative
	// the results can possible be.
	var opts github.SearchOptions
	opts.PerPage = 1
	repos, _, err := client.Search.Repositories(ctx, "language:Go", &opts)
	if err != nil {
		log.Fatalf("Could not complete initial request to count all go repositories: %v", err)
	}
	totalRepos := repos.GetTotal()
	log.Printf("total repositories: %d", totalRepos)

	// read in the set of files scraped so far to avoid persisting duplicates
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDONLY, 0666)
	if err != nil {
		log.Fatalf("could not open file %s: %v", filename, err)
	}
	visitedMap := ReadMap(file)
	log.Printf("%d repositories found already in %s", len(visitedMap), filename)
	err = file.Close()
	if err != nil {
		log.Fatalf("could not close file %s: %v", filename, err)
	}

	// generate the strings to search for. We will literally just run a search against GitHub for
	// golang repos containing this string and keep every unique result that's returned. This is
	// unfortunate, but Github doesn't really provide a good way to look at every repository which
	// has contents in a specific language.

	// TODO: rewrite this using the creation date to get all of the results.
	//       https://docs.github.com/en/free-pro-team@latest/github/searching-for-information-on-github/searching-for-repositories#search-by-when-a-repository-was-created-or-last-updated
	searchStrings := SearchStrings()

	for _, search := range searchStrings {
		if search < startKey {
			continue
		}
		// Each Github search provides the first 1000 results; we page in units of 100.
		for page := 0; page < 11; page++ {
			repos, incomplete, err := ScrapePage(ctx, client, search, page)

			// poor man's rate limiting support
			if limitErr, ok := err.(*github.RateLimitError); ok {
				sleepTime := limitErr.Rate.Reset.Time.Sub(time.Now())
				// wait until rate limit has reset, then retry
				log.Printf("hit rate limit; sleeping for %f seconds until %s", sleepTime.Seconds(), limitErr.Rate.Reset.Format(time.RFC822))
				time.Sleep(sleepTime)
				repos, incomplete, err = ScrapePage(ctx, client, search, page)
				if err != nil {
					log.Printf("Still hitting rate limit after reset; try again for page %d", page)
				}
			}

			if err != nil {
				log.Printf("error at page %d of search '%s': %v", page, search, err)
				break
			}
			if incomplete {
				log.Printf("had incomplete results for %s at page %d", search, page)
			} else {
				log.Printf("writing page %d for search %s", page, search)
			}
			WriteScrapeResults(visitedMap, repos, filename)
		}
		log.Printf("total of %d repositories (%f%%) found so far", len(visitedMap), 100*float32(len(visitedMap))/float32(totalRepos))
	}
}

// ReadMap extracts the set of repository IDs we already know about, to ensure we don't persist duplicates.
func ReadMap(file io.Reader) map[int64]struct{} {
	reader := csv.NewReader(file)
	result := make(map[int64]struct{})
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) == 0 {
			continue
		}
		id, err := strconv.ParseInt(record[0], 10, 64)
		if err != nil {
			log.Fatalf("could not parse first entry in record as 64-bit integer: %v", err)
		}
		result[id] = struct{}{}
	}
	return result
}

// WriteScrapeResults writes the set of repos to file, unless any repo's ID is already contained in the
// vistedMap.
func WriteScrapeResults(visitedMap map[int64]struct{}, repos []GoRepo, filename string) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR, 0666)
	defer file.Close()
	if err != nil {
		log.Fatalf("could not open file %v", err)
	}
	output := csv.NewWriter(file)
	var results [][]string
	for _, repo := range repos {
		if _, ok := visitedMap[repo.githubID]; ok {
			continue
		}
		results = append(results, []string{
			fmt.Sprintf("%d", repo.githubID),
			repo.gitURL,
			repo.githubURL,
			fmt.Sprintf("%s", repo.updatedAt.Format(time.RFC3339)),
		})
		visitedMap[repo.githubID] = struct{}{}
	}
	err = output.WriteAll(results)
	if err != nil {
		log.Fatalf("Could not write to file %s: %v", filename, err)
	}
}

// ScrapePage grabs one page of search results from Github's API. The provide search keyword is searched for, and
// the language is restricted to contain only results which contains files written in Go.
func ScrapePage(ctx context.Context, client *github.Client, search string, page int) ([]GoRepo, bool, error) {
	var opts github.SearchOptions
	opts.PerPage = pageSize
	opts.Page = page
	repos, _, err := client.Search.Repositories(ctx, "language:Go "+search, &opts)
	if err != nil {
		fmt.Println(err)
		return nil, false, err
	}
	if len(repos.Repositories) == 0 {
		return nil, false, fmt.Errorf("no repositories in page %d", page)
	}
	var result []GoRepo
	for _, repo := range repos.Repositories {
		result = append(result, GoRepo{
			repo.GetID(),
			repo.GetFullName(),
			repo.GetGitURL(),
			repo.GetURL(),
			repo.GetUpdatedAt().Time,
		})
	}
	return result, repos.GetIncompleteResults(), nil
}

// GoRepo is our internal representation of a Github repository containing Go code.
type GoRepo struct {
	githubID  int64
	fullname  string
	gitURL    string
	githubURL string
	updatedAt time.Time
}

// SearchStrings provides the strings to search for.
func SearchStrings() []string {
	// currently, we just check all pairs of letters. This suffices to obtain a sizable fraction
	// of repositories (11.6% so far!)
	result := make([]string, 26*26)
	i := 0
	for s := 'a'; s <= 'z'; s++ {
		for t := 'a'; t <= 'z'; t++ {
			result[i] = string(s) + string(t)
			i++
		}
	}
	return result
}
