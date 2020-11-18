package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"strings"
	"text/template"

	"github.com/google/go-github/github"
)

type IssueReporter struct {
	bot       *VetBot
	issueFile *MutexWriter
	csvWriter *csv.Writer
}

func NewIssueReporter(bot *VetBot, issueFile string) IssueReporter {
	return IssueReporter{
		bot: bot,
	}
}

func (ir *IssueReporter) ReportVetResult(result VetResult) {
	ir.bot.wg.Add(1)
	go func() {
		issueRequest := CreateIssueRequest(result)
		iss, _, err := ir.bot.client.Issues.Create(ir.bot.ctx, findingsOwner, findingsRepo, &issueRequest)
		if err != nil {
			log.Printf("error opening new issue: %v", err)
			return
		}
		// TODO: record this finding and issue number (and repo) as structured data somewhere.
		log.Printf("opened new issue at %s", iss.GetURL())
		ir.bot.wg.Done()
	}()
}

// TODO: link to README in the issues repo.
const IssueTemplate string = `
Found a possible issue in [{{.Repository.Owner}}/{{.Repository.Repo}}](https://www.github.com/{{.Repository.Owner}}/{{.Repository.Repo}}) at [{{.FilePath}}]({{.Link}})

The below snippet of Go code triggered static analysis which searches for goroutines and/or defer statements
which capture loop variables.

<details>
<summary>Click here to show the {{.SlocCount}} line(s) of Go.</summary>

~~~go
{{.Quote}}
~~~
</details>

commit ID: {{.RootCommitID}}
`

var parsed *template.Template

func init() {
	var err error
	parsed, err = template.New("issue").Parse(IssueTemplate)
	if err != nil {
		panic(err)
	}
}

// CreateIssueRequest writes the header and description of the GitHub issue which is opened with the result
// of any findings.
func CreateIssueRequest(result VetResult) github.IssueRequest {

	slocCount := result.End.Line - result.Start.Line
	title := fmt.Sprintf("%s/%s: %s; %d LoC", result.Owner, result.Repo, result.FilePath, slocCount)
	body := Description(result)
	fmt.Println(body)

	// TODO: labels based on lines of code
	return github.IssueRequest{
		Title: &title,
		Body:  &body,
	}
}

var tildifier *strings.Replacer = strings.NewReplacer("~", "`")

func Description(result VetResult) string {
	permalink := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", result.Owner, result.Repo, result.RootCommitID, result.Start.Filename, result.Start.Line, result.End.Line)
	quote := QuoteFinding(result)
	slocCount := result.End.Line - result.Start.Line

	buf := new(bytes.Buffer)
	err := parsed.Execute(buf, IssueResult{
		VetResult: result,
		Link:      permalink,
		Quote:     quote,
		SlocCount: slocCount,
	})
	if err != nil {
		log.Printf("could not create description: %v", err)
	}
	return tildifier.Replace(buf.String())
}

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

type IssueResult struct {
	VetResult
	Link      string
	Quote     string
	SlocCount int
}
