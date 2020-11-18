package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// 699280 Golang repositories are on GitHub

const pageSize = 100

var MinDate time.Time = time.Date(1970, 1, 1, 1, 1, 1, 0, time.UTC)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: go run main.go [outfile]")
		os.Exit(1)
	}

	filename := os.Args[1]

	// setup the OAuth2 client.
	ctx := context.Background()
	ghToken, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: ghToken},
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
	maxDateFound := ReadMaxDate(file)
	if maxDateFound.Before(MinDate) {
		maxDateFound = MinDate
	}
	err = file.Close() // dangerous! :o
	if err != nil {
		log.Fatalf("could not close file %s: %v", filename, err)
	}

	visitedMap := make(map[string]struct{})

	// TODO: split this up by month, then by day (as needed)... don't use the max time returned; it don't work.
	lastMaxDate := time.Now() // hack to ensure we go at least once around the loop
	for maxDateFound != lastMaxDate {
		lastMaxDate = maxDateFound
		// Each Github search provides the first 1000 results; we page in units of 100.
		for page := 0; page < 11; page++ {
			repos, _, err := ScrapePage(ctx, client, maxDateFound, page)

			// poor man's rate-limiting support
			if limitErr, ok := err.(*github.RateLimitError); ok {
				sleepTime := limitErr.Rate.Reset.Time.Sub(time.Now())
				// wait until rate limit has reset, then retry
				log.Printf("hit rate limit; sleeping for %f seconds until %s", sleepTime.Seconds(), limitErr.Rate.Reset.Format(time.RFC822))
				time.Sleep(sleepTime)
				repos, _, err = ScrapePage(ctx, client, maxDateFound, page)
				if err != nil {
					log.Printf("Still hitting rate limit after reset; try again for page %d", page)
				}
			}
			if err != nil {
				log.Printf("error at page %d: %v", page, err)
				break
			}
			for _, repo := range repos {
				if maxDateFound.Before(repo.CreatedAt) {
					maxDateFound = repo.CreatedAt
				}
			}
			WriteScrapeResults(visitedMap, repos, filename)
		}
		log.Printf("total of %d additional repositories (%f%%) found so far", len(visitedMap), 100*float32(len(visitedMap))/float32(totalRepos))
	}
}

// ReadMap extracts the set of repository IDs we already know about, to ensure we don't persist duplicates.
func ReadMaxDate(file io.Reader) time.Time {
	reader := csv.NewReader(file)
	var result time.Time
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) != 3 {
			continue
		}
		time, err := time.Parse(time.RFC3339, record[2])
		if err != nil {
			log.Fatalf("could not parse first entry in record as 64-bit integer: %v", err)
		}
		if result.Before(time) {
			result = time
		}
	}
	return result
}

// WriteScrapeResults writes the set of repos to file, unless any repo's ID is already contained in the
// vistedMap.
func WriteScrapeResults(visitedMap map[string]struct{}, repos []GoRepo, filename string) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_RDWR, 0666)
	defer file.Close()
	if err != nil {
		log.Fatalf("could not open file %v", err)
	}
	output := csv.NewWriter(file)
	var results [][]string
	for _, repo := range repos {
		if _, ok := visitedMap[repo.ID()]; ok {
			continue
		}
		results = append(results, []string{
			repo.Owner,
			repo.Repo,
			fmt.Sprintf("%s", repo.CreatedAt.Format(time.RFC3339)),
		})
		visitedMap[repo.ID()] = struct{}{}
	}
	err = output.WriteAll(results)
	if err != nil {
		log.Fatalf("Could not write to file %s: %v", filename, err)
	}
}

// ScrapePage grabs one page of search results from Github's API. The provide search keyword is searched for, and
// the language is restricted to contain only results which contains files written in Go.
func ScrapePage(ctx context.Context, client *github.Client, maxCreateDateKnown time.Time, page int) ([]GoRepo, bool, error) {
	var opts github.SearchOptions
	opts.PerPage = pageSize
	opts.Page = page
	dateStr := maxCreateDateKnown.Format("2006-01-02T15:04")
	fmt.Println(maxCreateDateKnown)
	repos, _, err := client.Search.Repositories(ctx, "language:Go sort:created-asc created:>"+dateStr, &opts)
	if err != nil {
		fmt.Println(err)
		return nil, false, err
	}
	if len(repos.Repositories) == 0 {
		return nil, false, fmt.Errorf("no repositories in page %d", page)
	}
	var result []GoRepo
	for _, repo := range repos.Repositories {
		nameTokens := strings.Split(repo.GetFullName(), "/")
		if len(nameTokens) != 2 {
			return nil, false, fmt.Errorf("undecipherable repository full name: %s", repo.GetFullName())
		}
		result = append(result, GoRepo{
			Owner:     nameTokens[0],
			Repo:      nameTokens[1],
			CreatedAt: repo.GetCreatedAt().Time,
		})
	}
	return result, repos.GetIncompleteResults(), nil
}

// GoRepo is our internal representation of a Github repository containing Go code.
type GoRepo struct {
	Owner     string
	Repo      string
	CreatedAt time.Time
}

func (gr GoRepo) ID() string {
	return gr.Owner + "/" + gr.Repo
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
