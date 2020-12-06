package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/github-vet/bots/internal/ratelimit"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

// MinExpertsNeededToClose controls the number of experts who must react before the issue is marked as closed.
const MinExpertsNeededToClose = 2

// ValidReactions lists the set of reactions which 'count' as input for the purpose of analysis.
var ValidReactions []string = []string{"+1", "-1", "rocket"}

// CloseTestIssues is a flag used to close any issues found which are in a test file.
var CloseTestIssues bool = true

// main runs trackbot. trackbot runs continuously, reading the entire issue tracker of a hardcoded GitHub repository
// every 15 minutes and updating its labels on the basis of community interactions.
//
// trackbot expects an environment variable named GITHUB_TOKEN which contains a valid personal access token used
// to authenticate with the GitHub API.
//
// trackbot expects read-write access to the working directory. It expects a single, non-empty file in the working
// directory named 'experts.csv'. This file should contain a list of github usernames followed by ",0", and a linebreak
// (unused at this time).
//
// trackbot creates two other files, 'issue_tracking.csv' and 'gophers.csv'. These files will be created if they do
// not exist. trackbot reads and writes to this file every time it polls.
//
// trackbot also creates a log file named 'MM-DD-YYYY.log', using the system date.
func main() {
	opts, err := parseOpts()
	if err != nil {
		log.Fatalf("error during config: %v", err)
	}

	log.Printf("configured options: %+v", opts)

	bot, err := NewTrackBot(opts)
	if err != nil {
		log.Fatalf("error creating trackbot: %v", err)
	}

	// run once at start
	bot.client.ResetCount()
	ProcessAllIssues(&bot)
	log.Printf("pass complete; performed %d API calls", bot.client.GetCount())

	// run continuously every poll interval
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	ticker := time.NewTicker(opts.PollFrequency)
	for {
		select {
		case <-ticker.C:
			bot.client.ResetCount()
			ProcessAllIssues(&bot)
			log.Printf("pass complete; performed %d API calls", bot.client.GetCount())
		case <-c:
			ticker.Stop()
			bot.wg.Wait()
			return
		}
	}
}

// ProcessAllIssues processes all pages of issues found in the provided repository and looks for updates.
func ProcessAllIssues(bot *TrackBot) {
	var err error
	bot.issues, err = ReadIssuesFile(bot.issueFilePath)
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
	issuePage, resp, err := bot.client.ListIssuesByRepo(bot.owner, bot.repo, &opts)
	for {
		if err != nil {
			log.Printf("could not grab issues: %v", err) // TODO: handle rate-limiting
		}
		ProcessIssuePage(bot, issuePage)
		opts.Page = resp.NextPage
		issuePage, resp, err = bot.client.ListIssuesByRepo(bot.owner, bot.repo, &opts)
		if resp.FirstPage == 0 { // no idea why this finds everything, but it does so consistently
			break
		}
	}
	err = WriteIssuesFile(bot.issueFilePath, bot.issues)
	if err != nil {
		log.Printf("could not write issues file: %v", err)
	}
	err = WriteGophersFile(bot.gopherFilePath, bot.gophers)
	if err != nil {
		log.Printf("could not write to gophers file: %v", err)
	}
}

// ProcessIssuePage processes one page of issues from GitHub.
func ProcessIssuePage(bot *TrackBot, issuePage []*github.Issue) {
	for _, issue := range issuePage {
		MaybeCloseIssueByLabel(bot, *issue)
		num := issue.GetNumber()
		if issue.GetReactions().GetTotalCount() == 0 {
			if !HasLabel(issue, "fresh") {
				bot.DoAsync(func() { AddLabel(bot, issue, "fresh") })
			}
			continue
		}
		if HasLabel(issue, "fresh") {
			bot.DoAsync(func() { RemoveLabel(bot, issue, "fresh") })
		}
		allReactions := GetAllReactions(bot, num)
		if record, ok := bot.issues[num]; ok {
			UpdateIssueReactions(bot, record, *issue, allReactions)
		} else {
			record := IssueFromGithub(issue)
			UpdateIssueReactions(bot, &record, *issue, allReactions)
			bot.issues[num] = &record
		}
	}
}

// MaybeCloseIssueByLabel closes the issue if it does not need to be considered. Trackbot does this since
// vetbot has no way to close an issue on its creation (due to apparent limitations in the GitHub API).
func MaybeCloseIssueByLabel(bot *TrackBot, issue github.Issue) {
	if !HasLabel(&issue, "test") && !HasLabel(&issue, "vendored") {
		return
	}
	bot.DoAsync(func() {
		state := "closed"
		req := github.IssueRequest{}
		req.State = &state
		_, _, err := bot.client.EditIssue(bot.owner, bot.repo, issue.GetNumber(), &req)
		if err != nil {
			log.Printf("could not mark test issue as closed: %v", err)
		}
	})
}

// GetAllReactions retrieves the set of reactions on an issue, paging through the API as needed to make sure they are all retrieved.
func GetAllReactions(bot *TrackBot, issueNum int) []*github.Reaction {
	var listOpts github.ListOptions
	listOpts.PerPage = 100
	reactions, resp, err := bot.client.ListIssueReactions(bot.owner, bot.repo, issueNum, &listOpts)
	var allReactions []*github.Reaction
	for {
		if err != nil {
			log.Printf("could not read reaction page: %v", err)
			break
		}
		allReactions = append(allReactions, reactions...)
		listOpts.Page = resp.NextPage
		reactions, resp, err = bot.client.ListIssueReactions(bot.owner, bot.repo, issueNum, &listOpts)
		if resp.FirstPage == 0 {
			break
		}
	}
	return allReactions
}

// UpdateIssueReactions updates the set of reactions associated with a single issue.
func UpdateIssueReactions(bot *TrackBot, record *Issue, issue github.Issue, allReactions []*github.Reaction) {
	expertAssessments := make(map[string]int)
	var expertUsernames []string
	for _, r := range allReactions {
		if !isValidReaction(r.GetContent()) {
			continue
		}
		username := r.GetUser().GetLogin()
		if exp, ok := bot.experts[username]; ok {
			expertAssessments[r.GetContent()]++
			exp.AssessmentCount++
			expertUsernames = append(expertUsernames, username)
		}
	}
	if len(expertAssessments) == 0 {
		UpdateCommunityAssessment(bot, record, &issue, allReactions)
		return // no expert has chimed in yet.
	}
	if len(expertAssessments) > 1 {
		HandleExpertDisagreement(bot, record, &issue, expertUsernames, expertAssessments)
		return
	}
	if len(expertAssessments) != 1 {
		panic("there should only be one entry in expertAssessments at this point! someone broke the code!")
	}
	for assessment, expertCount := range expertAssessments {
		HandleExpertAgreement(bot, record, &issue, allReactions, assessment, expertCount)
	}
}

// ConfusionThreshold marks the fraction of total score applied to an isssue needed before the community is considered 'confused'.
const ConfusionThreshold = 0.8

// CommunityScoreThreshold marks the minimum reliability score needed on an issue before it will have a community label applied.
const CommunityScoreThreshold = 1.0

// HighCommunityScoreThreshold marks the threshold needed before the 'reliable' label is applied.
const HighCommunityScoreThreshold = 5.0

// UpdateCommunityAssessment updates the overall community assessment based on the reliability of all the users involved.
func UpdateCommunityAssessment(bot *TrackBot, record *Issue, issue *github.Issue, reactions []*github.Reaction) {
	scores := make(map[string]float32)
	for _, r := range reactions {
		if !isValidReaction(r.GetContent()) {
			continue
		}
		scores[r.GetContent()] += Score(bot.gophers, r.GetUser().GetLogin())
	}
	totalScore := float32(0)
	for _, score := range scores {
		totalScore += score
	}
	// find the outomce with the highest score.
	maxScore := float32(0)
	var maxOutcome string
	for outcome, score := range scores {
		if score > CommunityScoreThreshold && score > maxScore {
			maxScore = score
			maxOutcome = outcome
		}
	}
	if maxOutcome != "" {
		if maxScore < ConfusionThreshold {
			bot.DoAsync(func() { SetCommunityLabel(bot, issue, "confused") })
		} else {
			bot.DoAsync(func() { SetCommunityLabel(bot, issue, maxOutcome) })
			if maxScore > HighCommunityScoreThreshold {
				bot.DoAsync(func() { AddLabel(bot, issue, "reliable") })
			} else if HasLabel(issue, "reliable") {
				bot.DoAsync(func() { RemoveLabel(bot, issue, "reliable") })
			}
		}
	}
}

// HandleExpertAgreement handles the case where all the experts who have weighed in on the issue agree.
func HandleExpertAgreement(bot *TrackBot, record *Issue, issue *github.Issue, reactions []*github.Reaction, assessment string, numExperts int) {
	if !isValidReaction(assessment) {
		return
	}
	if record.ExpertAssessment == "" {
		log.Printf("experts agree! issue %d is %s\n", record.Number, assessment)
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
	if record.DisagreeFlag {
		record.DisagreeFlag = false
	}

	bot.DoAsync(func() { SetExpertLabel(bot, issue, assessment) })
	bot.DoAsync(func() { MaybeCloseIssue(bot, record, numExperts) })
}

// MaybeCloseIssue closes the issue if the number of experts who have provided their assessment exceeds the threshold.
func MaybeCloseIssue(bot *TrackBot, record *Issue, expertCount int) {
	if expertCount < MinExpertsNeededToClose {
		return
	}
	state := "closed"
	req := github.IssueRequest{
		State: &state,
	}
	_, _, err := bot.client.EditIssue(bot.owner, bot.repo, record.Number, &req)
	if err != nil {
		log.Printf("could not close issue %d after %d experts agreed", record.Number, expertCount)
	}
}

// HasLabel returns true if the issue has a matching label.
func HasLabel(issue *github.Issue, label string) bool {
	for _, l := range issue.Labels {
		if l.GetName() == label {
			return true
		}
	}
	return false
}

// AddLabel adds the provided label to the issue, if it is present.
func AddLabel(bot *TrackBot, issue *github.Issue, label string) {
	if HasLabel(issue, label) {
		return // avoid API call
	}
	_, _, err := bot.client.AddLabelsToIssue(bot.owner, bot.repo, issue.GetNumber(), []string{label})
	if err != nil {
		log.Printf("could not label issue %d with label %s: %v", issue.GetNumber(), label, err)
	}
}

// RemoveLabel removes the provided label from the issue, if it is present.
func RemoveLabel(bot *TrackBot, issue *github.Issue, label string) {
	if !HasLabel(issue, label) {
		return // avoid API call
	}
	_, err := bot.client.RemoveLabelForIssue(bot.owner, bot.repo, issue.GetNumber(), label)
	if err != nil {
		log.Printf("could not remove label %s from issue %d: %v", label, issue.GetNumber(), err)
	}
}

// SetExpertLabel adds or overwrites the expert label associated with the provided assessment.
func SetExpertLabel(bot *TrackBot, issue *github.Issue, assessment string) {
	newLabels, changed := modifyLabels(issue.Labels, "experts", assessment)
	if !changed {
		return // avoid extra API calls
	}
	_, _, err := bot.client.ReplaceLabelsForIssue(bot.owner, bot.repo, issue.GetNumber(), newLabels)
	if err != nil {
		log.Printf("could not label issue %d with expert assessment %s", issue.GetNumber(), assessment)
	}
}

// SetCommunityLabel adds or overwrites the community label associated with the provided assessment.
func SetCommunityLabel(bot *TrackBot, issue *github.Issue, assessment string) {
	newLabels, changed := modifyLabels(issue.Labels, "community", assessment)
	if !changed {
		return // avoid extra API calls
	}
	_, _, err := bot.client.ReplaceLabelsForIssue(bot.owner, bot.repo, issue.GetNumber(), newLabels)
	if err != nil {
		log.Printf("could not label issue %d with community assessment %s", issue.GetNumber(), assessment)
	}
}

// modifyLabels returns the modified set of labels, and a flag indicating whether any labels were changed.
func modifyLabels(labels []*github.Label, prefix, assessment string) ([]string, bool) {
	var result []string
	newLabel := prefix + ": :" + assessment + ":"
	flag := true
	for _, label := range labels {
		if !strings.HasPrefix(label.GetName(), prefix) {
			result = append(result, label.GetName())
		} else {
			flag = label.GetName() != newLabel
		}
	}
	result = append(result, newLabel)
	return result, flag
}

func isValidReaction(reaction string) bool {
	for _, str := range ValidReactions {
		if str == reaction {
			return true
		}
	}
	return false
}

// DisagreementTemplate is the template used to comment when experts disagree on the outcome of an issue.
const DisagreementTemplate string = `
Detected disagreement among experts! {{range $username := .Usernames }} @{{$username}} {{end}} please discuss.

Expert votes:
{{range $outcome, $count := .VoteCounts}}
:{{$outcome}}: = {{$count}}
{{end}}
`

var parsed *template.Template

func init() {
	var err error
	parsed, err = template.New("disagreement").Parse(DisagreementTemplate)
	if err != nil {
		panic(err)
	}
}

// DisagreementData describes data for the Disagreement template.
type DisagreementData struct {
	Usernames  []string
	VoteCounts map[string]int
}

// HandleExpertDisagreement handles the case where experts who have weighed in on the issue do not agree.
func HandleExpertDisagreement(bot *TrackBot, record *Issue, issue *github.Issue, expertsToThrottle []string, expertAssessments map[string]int) {
	if record.DisagreeFlag {
		return // no need to act again
	}
	record.DisagreeFlag = true
	log.Printf("experts disagree! %v\n", expertsToThrottle)

	bot.DoAsync(func() {
		SetExpertLabel(bot, issue, "confused")
	})
	bot.DoAsync(func() {
		ThrottleExperts(bot, record, expertsToThrottle, expertAssessments)
	})
}

// ThrottleExperts posts a comment on the issue mentioning the experts to draw attention to their disagreement and start a
// transparent conversation.
func ThrottleExperts(bot *TrackBot, record *Issue, expertsToThrottle []string, expertAssessments map[string]int) {
	var b strings.Builder
	err := parsed.Execute(&b, DisagreementData{
		Usernames:  expertsToThrottle,
		VoteCounts: expertAssessments,
	})
	if err != nil {
		log.Printf("could not execute disagreement template: %v", err)
		return
	}
	var comment github.IssueComment
	body := b.String()
	comment.Body = &body
	_, _, err = bot.client.CreateIssueComment(bot.owner, bot.repo, record.Number, &comment)
	if err != nil {
		log.Printf("could not post issue disagreement comment: %v", err)
	}
}

type TrackBot struct {
	client         *ratelimit.Client
	wg             sync.WaitGroup
	issueFilePath  string
	gopherFilePath string
	owner          string
	repo           string
	gophers        map[string]*Gopher
	issues         map[int]*Issue
	experts        map[string]*Expert
}

func (b *TrackBot) DoAsync(f func()) {
	b.wg.Add(1)
	go func(f func()) {
		f()
		b.wg.Done()
	}(f)
}

func NewTrackBot(opts opts) (TrackBot, error) {
	experts, err := ReadExpertsFile(opts.ExpertsFile)
	if err != nil {
		log.Fatalf("cannot read experts file: %v", err)
		return TrackBot{}, err
	}
	if len(experts) == 0 {
		return TrackBot{}, errors.New("refusing to start track bot with an empty list of experts")
	}
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: string(opts.GithubToken)},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	limited, err := ratelimit.NewClient(ctx, client)
	if err != nil {
		log.Fatalf("cannot create ratelimited client: %v", err)
		return TrackBot{}, err
	}
	return TrackBot{
		client:         &limited,
		issueFilePath:  opts.TrackingFile,
		gopherFilePath: opts.GophersFile,
		experts:        experts,
		owner:          opts.Owner,
		repo:           opts.Repo,
	}, nil
}
