package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/google/go-github/github"
)

// IssueReporter reports issues and maintains a local csv file to document any issues opened.
type IssueReporter struct {
	bot       *VetBot
	issueFile *MutexWriter
	csvWriter *csv.Writer
}

// NewIssueReporter constructs a new issue reporter with the provided bot. The issue file will be
// created if it doesn't already exist. It stores a list of issues which have already been opened.
func NewIssueReporter(bot *VetBot, issueFile string) (*IssueReporter, error) {
	issueWriter, err := os.OpenFile(issueFile, os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	mw := NewMutexWriter(issueWriter)
	return &IssueReporter{
		bot:       bot,
		issueFile: &mw,
		csvWriter: csv.NewWriter(&mw),
	}, nil
}

func (ir *IssueReporter) Close() error {
	return ir.issueFile.Close()
}

// ReportVetResult asynchronously creates a new GitHub issue to report the findings of the VetResult.
func (ir *IssueReporter) ReportVetResult(result VetResult) {
	ir.bot.wg.Add(1)
	go func() {
		issueRequest := CreateIssueRequest(result)
		iss, _, err := ir.bot.client.Issues.Create(ir.bot.ctx, findingsOwner, findingsRepo, &issueRequest)
		if err != nil {
			log.Printf("error opening new issue: %v", err)
			return
		}
		ir.writeIssue(result, iss)
		log.Printf("opened new issue at %s", iss.GetHTMLURL())
		ir.bot.wg.Done()
	}()
}

func (ir *IssueReporter) writeIssue(result VetResult, iss *github.Issue) error {
	issueNum := fmt.Sprintf("%d", iss.GetNumber())
	err := ir.csvWriter.Write([]string{findingsOwner, findingsRepo, issueNum})
	ir.csvWriter.Flush()
	if err != nil {
		return err
	}
	return nil
}

// CreateIssueRequest writes the header and description of the GitHub issue which is opened with the result
// of any findings.
func CreateIssueRequest(result VetResult) github.IssueRequest {

	slocCount := result.End.Line - result.Start.Line
	title := fmt.Sprintf("%s/%s: %s; %d LoC", result.Owner, result.Repo, result.FilePath, slocCount)
	body := Description(result)

	// TODO: labels based on lines of code
	return github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
}

// Description writes the description of an issue, given a VetResult.
func Description(result VetResult) string {
	permalink := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", result.Owner, result.Repo, result.RootCommitID, result.Start.Filename, result.Start.Line, result.End.Line)
	quote := QuoteFinding(result)
	slocCount := result.End.Line - result.Start.Line

	var b strings.Builder
	err := parsed.Execute(&b, IssueResult{
		VetResult: result,
		Link:      permalink,
		Quote:     quote,
		SlocCount: slocCount,
	})
	if err != nil {
		log.Printf("could not create description: %v", err)
	}
	return b.String()
}

// QuoteFinding retrieves the snippet of code that caused the VetResult.
func QuoteFinding(result VetResult) string {
	lineStart, lineEnd := result.Start.Line, result.End.Line
	sc := bufio.NewScanner(bytes.NewReader(result.FileContents))
	line := 0
	var sb strings.Builder
	for sc.Scan() && line < lineEnd {
		line++
		if lineStart <= line && line <= lineEnd {
			sb.WriteString(sc.Text() + "\n")
		}
	}
	return sb.String()
}

// IssueResult enriches a VetResult with some additional information.
type IssueResult struct {
	VetResult
	Link      string
	Quote     string
	SlocCount int
}

// IssueResultTemplate is the template used to file a GitHub issue. It's meant to be invoked with an
// IssueResult.
// TODO: link to the README in the issues repository for more information.
var IssueResultTemplate string = `
Found a possible issue in [{{.Repository.Owner}}/{{.Repository.Repo}}](https://www.github.com/{{.Repository.Owner}}/{{.Repository.Repo}}) at [{{.FilePath}}]({{.Link}})

The below snippet of Go code triggered static analysis which searches for goroutines and/or defer statements
which capture loop variables.

<details>
<summary>Click here to show {{.SlocCount}} line(s) of Go.</summary>

~~~go
{{.Quote}}
~~~
</details>

commit ID: {{.RootCommitID}}
`

var parsed *template.Template

func init() {
	IssueResultTemplate = strings.NewReplacer("~", "`").Replace(IssueResultTemplate)
	var err error
	parsed, err = template.New("issue").Parse(IssueResultTemplate)
	if err != nil {
		panic(err)
	}
}
