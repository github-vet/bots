package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// TODO: maybe don't hardcode these? We don't have any reason to do otherwise, though.
const findingsOwner = "github-vet"
const findingsRepo = "rangeclosure-findings"

func main() {
	logFilename := time.Now().Format("01-02-2006") + ".log"
	logFile, err := os.OpenFile(logFilename, os.O_APPEND|os.O_CREATE, 0666)
	defer logFile.Close()
	if err != nil {
		log.Fatalf("cannot open log file for writing: %v", err)
	}
	log.SetOutput(logFile)

	ghToken, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatalln("could not find GITHUB_TOKEN environment variable")
	}
	bot, err := NewTrackBot(ghToken, "issue_tracking.csv", "experts.csv", "gophers.csv")
	if err != nil {
		log.Fatalf("error creating trackbot: %v", err)
	}

	ProcessAllIssues(&bot)
}

// ProcessAllIssues processes all pages of issues found in the provided repository.
func ProcessAllIssues(bot *TrackBot) {
	issues, err := ReadIssuesFile(bot.issueFilePath)
	if err != nil {
		log.Fatalln("could not read from issues file")
		return
	}
	bot.gophers, err = ReadGophersFile(bot.gopherFilePath)
	if err != nil {
		log.Fatalln("could not read from gophers file")
	}
	var opts github.IssueListByRepoOptions
	opts.PerPage = 100
	issuePage, resp, err := bot.client.Issues.ListByRepo(bot.ctx, findingsOwner, findingsRepo, &opts)
	for {
		if err != nil {
			log.Printf("could not grab issues: %v", err) // TODO: handle rate-limiting
		}
		ProcessIssuePage(bot, issues, issuePage)
		opts.Page = resp.NextPage
		issuePage, resp, err = bot.client.Issues.ListByRepo(bot.ctx, findingsOwner, findingsRepo, &opts)
		if resp.FirstPage == 0 { // no idea why this finds everything, but it does so consistently
			break
		}
	}
	err = WriteIssuesFile(bot.issueFilePath, issues)
	if err != nil {
		log.Printf("could not write issues file: %v", err)
	}
	err = WriteGophersFile(bot.gopherFilePath, bot.gophers)
	if err != nil {
		log.Printf("could not write to gophers file: %v", err)
	}
}

// TODO: manage state better without passing into arguments.

// ProcessIssuePage processes one page of issues from GitHub.
func ProcessIssuePage(bot *TrackBot, issueRecords map[int]*Issue, issuePage []*github.Issue) {
	for _, issue := range issuePage {
		num := issue.GetNumber()
		if issue.GetReactions().GetTotalCount() == 0 {
			// TODO: add 'new' label
			continue
		}
		allReactions := GetAllReactions(bot, num)
		if record, ok := issueRecords[num]; ok {
			UpdateIssueReactions(bot, record, allReactions)
		} else {
			record := IssueFromGithub(issue)
			UpdateIssueReactions(bot, &record, allReactions)
			issueRecords[num] = &record
		}
	}
}

// GetAllReactions retrieves the set of reactions on an issue, paging through the API as needed.
func GetAllReactions(bot *TrackBot, issueNum int) []*github.Reaction {
	var listOpts github.ListOptions
	listOpts.PerPage = 100
	reactions, resp, err := bot.client.Reactions.ListIssueReactions(bot.ctx, findingsOwner, findingsRepo, issueNum, &listOpts)
	var allReactions []*github.Reaction
	for {
		if err != nil {
			log.Printf("could not read issues") // TODO: handle rate-limiting
		}
		allReactions = append(allReactions, reactions...)
		listOpts.Page = resp.NextPage
		reactions, resp, err = bot.client.Reactions.ListIssueReactions(bot.ctx, findingsOwner, findingsRepo, issueNum, &listOpts)
		if resp.FirstPage == 0 {
			break
		}
	}
	return allReactions
}

// UpdateIssueReactions updates the set of reactions associated with a single issue.
func UpdateIssueReactions(bot *TrackBot, record *Issue, allReactions []*github.Reaction) {
	expertAssessments := make(map[string]int)
	var expertUsernames []string
	for _, r := range allReactions {
		username := r.GetUser().GetLogin()
		if exp, ok := bot.experts[username]; ok {
			expertAssessments[r.GetContent()]++
			exp.AssessmentCount++
			expertUsernames = append(expertUsernames, username)
		}
	}
	if len(expertAssessments) == 0 {
		UpdateCommunityAssessment(bot, record, allReactions)
		return // no expert has chimed in yet.
	}
	if len(expertAssessments) > 1 {
		HandleExpertDisagreement(bot, record, expertUsernames, expertAssessments)
	}
	if len(expertAssessments) != 1 {
		panic("there should only be one entry in expertAssessments! someone broke the code!")
	}
	for assessment := range expertAssessments {
		HandleExpertAgreement(bot, record, allReactions, assessment)
	}
}

// UpdateCommunityAssessment updates the overall community assessment based on the reliability of all the users involved.
func UpdateCommunityAssessment(bot *TrackBot, record *Issue, reactions []*github.Reaction) {
	scores := make(map[string]float32)
	for _, r := range reactions {
		scores[r.GetContent()] += Score(bot.gophers, r.GetUser().GetLogin())
	}
	for outcome, score := range scores {
		fmt.Printf("%d has score %f for %s\n", record.Number, score, outcome)
	}
}

const MinExpertsNeededToClose = 2

// HandleExpertAgreement handles the case where all the experts who have weighed in on the issue agree.
func HandleExpertAgreement(bot *TrackBot, record *Issue, reactions []*github.Reaction, assessment string) {
	log.Printf("experts agree! issue %d is %s\n", record.Number, assessment)
	if record.ExpertAssessment == "" {
		for _, r := range reactions {
			username := r.GetUser().GetLogin()
			if _, ok := bot.experts[username]; ok {
				continue
			}
			if _, ok := bot.gophers[username]; !ok {
				bot.gophers[username] = &Gopher{
					Username: username,
				}
			}
			bot.gophers[username].Assessments++
			if r.GetContent() != assessment {
				bot.gophers[username].Disagreements++
			}
		}
	}
	record.ExpertAssessment = assessment
	// TODO: label the issue with the expert assessment.
	// TODO: if enough experts agree, close the issue.
}

// HandleExpertDisagreement handles the case where experts who have weighed in on the issue do not agree.
func HandleExpertDisagreement(bot *TrackBot, record *Issue, expertsToThrottle []string, expertAssessments map[string]int) {
	log.Printf("experts disagree! %v\n", expertsToThrottle)
	// TODO: post an issue comment highlighting the disagreement and @ing the experts involved to make them aware that a transparent discussion ought to be had.
	// TOOD: label the issue "experts-disagree"
}

type TrackBot struct {
	ctx            context.Context
	client         *github.Client
	wg             sync.WaitGroup
	issueFilePath  string
	gopherFilePath string
	gophers        map[string]*Gopher
	experts        map[string]*Expert
}

func NewTrackBot(token string, issueFilePath, expertsFilePath string, gopherFilePath string) (TrackBot, error) {
	experts, err := ReadExpertsFile(expertsFilePath)
	if err != nil {
		log.Fatalf("cannot read experts file: %v", err)
		return TrackBot{}, err
	}
	if len(experts) == 0 {
		return TrackBot{}, errors.New("refusing to start track bot with an empty list of experts")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(token)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	return TrackBot{
		ctx:            ctx,
		client:         client,
		issueFilePath:  issueFilePath,
		gopherFilePath: gopherFilePath,
		experts:        experts,
	}, nil
}
