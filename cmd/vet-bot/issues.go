package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/google/go-github/v32/github"
)

// Md5Checksum represents an MD5 checksum as per the standard library.
type Md5Checksum [md5.Size]byte

// IssueReporter reports issues and maintains a local csv file to document any issues opened.
type IssueReporter struct {
	bot       *VetBot
	issueFile *MutexWriter
	csvWriter *csv.Writer
	md5s      map[Md5Checksum]struct{} // hashes of the code reported to protect against vendored / duplicated code
	owner     string
	repo      string
}

// NewIssueReporter constructs a new issue reporter with the provided bot. The issue file will be
// created if it doesn't already exist. It stores a list of issues which have already been opened.
func NewIssueReporter(bot *VetBot, issueFile string, owner, repo string) (*IssueReporter, error) {
	md5s, err := readMd5s(issueFile)
	if err != nil {
		return nil, err
	}

	issueWriter, err := os.OpenFile(issueFile, os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	mw := NewMutexWriter(issueWriter)
	return &IssueReporter{
		bot:       bot,
		issueFile: &mw,
		csvWriter: csv.NewWriter(&mw),
		md5s:      md5s,
		owner:     owner,
		repo:      repo,
	}, nil
}

// Close closes the underlying issue file.
func (ir *IssueReporter) Close() error {
	return ir.issueFile.Close()
}

func readMd5s(filename string) (map[Md5Checksum]struct{}, error) {
	result := make(map[Md5Checksum]struct{})
	if _, err := os.Stat(filename); err != nil {
		return result, nil
	}
	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(file)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) != 4 {
			log.Printf("malformed line in repository list %s", filename)
			continue
		}
		var md5Sum Md5Checksum
		base64.StdEncoding.Decode(md5Sum[:], []byte(record[3]))
		result[md5Sum] = struct{}{}
	}
	return result, nil
}

// ReportVetResult asynchronously creates a new GitHub issue to report the findings of the VetResult.
func (ir *IssueReporter) ReportVetResult(result VetResult) {
	md5Sum := md5.Sum(result.FileContents)
	if _, ok := ir.md5s[md5Sum]; ok {
		log.Printf("found duplicated code in %s", result.FilePath)
		return
	}
	ir.md5s[md5Sum] = struct{}{}

	ir.bot.wg.Add(1)
	go func(result VetResult) {
		issueRequest := CreateIssueRequest(result)
		iss, _, err := ir.bot.client.CreateIssue(ir.owner, ir.repo, &issueRequest)
		if err != nil {
			log.Printf("error opening new issue: %v", err)
			return
		}
		ir.writeIssueToFile(result, iss)
		log.Printf("opened new issue at %s", iss.GetHTMLURL())
		ir.bot.wg.Done()
	}(result)
}

func (ir *IssueReporter) writeIssueToFile(result VetResult, iss *github.Issue) error {
	issueNum := fmt.Sprintf("%d", iss.GetNumber())
	md5Sum := md5.Sum(result.FileContents)
	md5Str := base64.StdEncoding.EncodeToString(md5Sum[:])
	err := ir.csvWriter.Write([]string{ir.owner, ir.repo, issueNum, md5Str})
	ir.csvWriter.Flush()
	if err != nil {
		return err
	}
	return nil
}

// CreateIssueRequest writes the header and description of the GitHub issue which is opened with the result
// of any findings.
func CreateIssueRequest(result VetResult) github.IssueRequest {

	slocCount := result.End.Line - result.Start.Line + 1
	title := fmt.Sprintf("%s/%s: %s; %d LoC", result.Owner, result.Repo, result.FilePath, slocCount)
	body := Description(result)
	labels := Labels(result)
	state := State(result)

	// TODO: labels based on lines of code
	return github.IssueRequest{
		Title:  &title,
		Body:   &body,
		Labels: &labels,
		State:  &state,
	}
}

// Description writes the description of an issue, given a VetResult.
func Description(result VetResult) string {
	permalink := fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s#L%d-L%d", result.Owner, result.Repo, result.RootCommitID, result.Start.Filename, result.Start.Line, result.End.Line)
	quote := QuoteFinding(result)
	slocCount := result.End.Line - result.Start.Line + 1

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

// Labels returns the list of labels to be applied to a VetResult.
func Labels(result VetResult) []string {
	slocCount := result.End.Line - result.Start.Line
	labels := []string{"fresh"}
	if slocCount < 10 {
		labels = append(labels, "tiny")
	} else if slocCount < 50 {
		labels = append(labels, "small")
	} else if slocCount < 100 {
		labels = append(labels, "medium")
	} else if slocCount < 250 {
		labels = append(labels, "large")
	} else {
		labels = append(labels, "huge")
	}
	if strings.HasSuffix(result.FilePath, "_test.go") {
		labels = append(labels, "test")
	}
	if strings.HasPrefix(result.FilePath, "vendor/") {
		labels = append(labels, "vendored")
	}
	return labels
}

func State(result VetResult) string {
	if strings.HasSuffix(result.FilePath, "_test.go") {
		return "closed"
	}
	return "open"
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

Below is the message reported by the analyzer for this snippet of code. Beware that the analyzer only reports the first
issue it finds, so please do not limit your consideration to the contents of the below message.

> {{.Message}}

[Click here to see the code in its original context.]({{.Link}})

<details>
<summary>Click here to show the {{.SlocCount}} line(s) of Go which triggered the analyzer.</summary>

~~~go
{{.Quote}}
~~~
</details>

Leave a reaction on this issue to contribute to the project by classifying this instance as a **Bug** :-1:, **Mitigated** :+1:, or **Desirable Behavior** :rocket:
See the descriptions of the classifications [here](https://github.com/github-vet/rangeclosure-findings#how-can-i-help) for more information.

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
