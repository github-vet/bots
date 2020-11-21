package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// MinExpertsNeededToClose controls the number of experts who must react before the issue is marked as closed.
const MinExpertsNeededToClose = 2

// ValidReactions lists the set of reactions which 'count' as input for the purpose of analysis.
var ValidReactions []string = []string{"+1", "-1", "rocket"}

// TODO: maybe don't hardcode these? We don't have any reason to do otherwise right now, though.
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
	issuePage, resp, err := bot.client.Issues.ListByRepo(bot.ctx, findingsOwner, findingsRepo, &opts)
	for {
		if err != nil {
			log.Printf("could not grab issues: %v", err) // TODO: handle rate-limiting
		}
		ProcessIssuePage(bot, issuePage)
		opts.Page = resp.NextPage
		issuePage, resp, err = bot.client.Issues.ListByRepo(bot.ctx, findingsOwner, findingsRepo, &opts)
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
		num := issue.GetNumber()
		if issue.GetReactions().GetTotalCount() == 0 {
			// TODO: add 'new' label
			continue
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

// GetAllReactions retrieves the set of reactions on an issue, paging through the API as needed to make sure they are all retrieved.
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

// ConfusionThreshold marks the fraction of the score needed before the community is considered 'confused'.
const ConfusionThreshold = 0.8

// CommunityReliabilityThreshold marks the minimum reliability score needed on an issue before it will have a community label applied.
const CommunityReliabilityThreshold = 1.0

// HighCommunityReliabilityThreshold marks the threshold needed before the 'high reliability' label is applied.
const HighCommunityReliabilityThreshold = 5.0

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
	maxScore := float32(0)
	var maxOutcome string
	for outcome, score := range scores {
		if score > CommunityReliabilityThreshold && score > maxScore {
			maxScore = score
			maxOutcome = outcome
		}
	}
	if maxOutcome != "" {
		if maxScore < ConfusionThreshold {
			go SetCommunityLabel(bot, issue, "confused")
		} else {
			go SetCommunityLabel(bot, issue, maxOutcome)
			if maxScore > HighCommunityReliabilityThreshold {

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

	go SetExpertLabel(bot, issue, assessment)
	go MaybeCloseIssue(bot, record, numExperts)
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
	_, _, err := bot.client.Issues.Edit(bot.ctx, findingsOwner, findingsRepo, record.Number, &req)
	if err != nil {
		log.Printf("could not close issue %d after %d experts agreed", record.Number, expertCount)
	}
}

// AddLabel adds the provided label to the issue, if it is present.
func AddLabel(bot *TrackBot, issue *github.Issue, label string) {
	hasLabel := false
	for _, l := range issue.Labels {
		if l.GetName() == label {
			hasLabel = true
		}
	}
	if hasLabel {
		return // avoid API call
	}
	_, _, err := bot.client.Issues.AddLabelsToIssue(bot.ctx, findingsOwner, findingsRepo, issue.GetNumber(), []string{label})
	if err != nil {
		log.Printf("could not label issue %d with label %s: %v", issue.GetNumber(), label, err)
	}
}

// RemoveLabel removes the provided label from the issue, if it is present.
func RemoveLabel(bot *TrackBot, issue *github.Issue, label string) {
	hasLabel := false
	for _, l := range issue.Labels {
		if l.GetName() == label {
			hasLabel = true
		}
	}
	if !hasLabel {
		return // avoid API call
	}
	_, err := bot.client.Issues.RemoveLabelForIssue(bot.ctx, findingsOwner, findingsRepo, issue.GetNumber(), label)
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
	_, _, err := bot.client.Issues.ReplaceLabelsForIssue(bot.ctx, findingsOwner, findingsRepo, issue.GetNumber(), newLabels)
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
	_, _, err := bot.client.Issues.ReplaceLabelsForIssue(bot.ctx, findingsOwner, findingsRepo, issue.GetNumber(), newLabels)
	if err != nil {
		log.Printf("could not label issue %d with community assessment %s", issue.GetNumber(), assessment)
	}
}

// modifyLabels returns the modified set of labels, and a flag indicating whether any labels were changed.
func modifyLabels(labels []github.Label, prefix, assessment string) ([]string, bool) {
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

	go SetExpertLabel(bot, issue, "confused")
	go ThrottleExperts(bot, record, expertsToThrottle, expertAssessments)
	// TODO: post a comment on the issue highlighting any disagreement and @ing the experts involved to make them aware that a transparent discussion ought to be had.
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
	_, _, err = bot.client.Issues.CreateComment(bot.ctx, findingsOwner, findingsRepo, record.Number, &comment)
	if err != nil {
		log.Printf("could not post issue disagreement comment: %v", err)
	}
}

type TrackBot struct {
	ctx            context.Context
	client         *github.Client
	wg             sync.WaitGroup
	issueFilePath  string
	gopherFilePath string
	gophers        map[string]*Gopher
	issues         map[int]*Issue
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

var parsed *template.Template

func init() {
	var err error
	parsed, err = template.New("disagreement").Parse(DisagreementTemplate)
	if err != nil {
		panic(err)
	}
}
