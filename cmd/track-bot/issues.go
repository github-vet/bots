package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/google/go-github/v32/github"
)

const issueNumFields int = 3

// Issue is a local record of an issue being tracked by this bot.
type Issue struct {
	// Number records the issue number.
	Number int
	// ExpertAssessment indicates the assessment of experts.
	ExpertAssessment string
	// DisagreeFlag is true when this issue has been commented on for disagreement.
	DisagreeFlag bool
}

// HasExpertAssessment is true if the issue has already been expertly assessed
func (i Issue) HasExpertAssessment() bool {
	return i.ExpertAssessment != ""
}

// WriteIssuesFile writes the map of issues to the provided file, truncating its
// contents in the process.
func WriteIssuesFile(path string, issues map[int]*Issue) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	for _, iss := range issues {
		writer.Write(csvLineFromIssue(*iss))
	}
	writer.Flush()
	return nil
}

// ReadIssuesFile reads a map of issues keyed by issue ID from the provided file.
func ReadIssuesFile(path string) (map[int]*Issue, error) {
	result := make(map[int]*Issue)
	if _, err := os.Stat(path); err != nil {
		return result, nil
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	defer file.Close()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(file)
	lineNum := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lineNum++
		if len(record) != issueNumFields {
			log.Printf("malformed line in issues list %s line %d, expected %d fields, found %d", path, lineNum, issueNumFields, len(record))
			continue
		}
		issue, err := issueFromCsvLine(record)
		if err != nil {
			log.Printf("malformed line in issues list %s line %d: %v", path, lineNum, err)
		}
		result[issue.Number] = &issue
	}
	return result, nil
}

func issueFromGithub(iss *github.Issue) Issue {
	return Issue{
		Number:       iss.GetNumber(),
		DisagreeFlag: false,
	}
}

func issueFromCsvLine(line []string) (Issue, error) {
	id, err := strconv.ParseInt(line[0], 10, 32)
	if err != nil {
		return Issue{}, err
	}
	disagree, err := strconv.ParseBool(line[2])
	if err != nil {
		return Issue{}, err
	}
	return Issue{
		Number:           int(id),
		ExpertAssessment: line[1],
		DisagreeFlag:     disagree,
	}, nil
}

func csvLineFromIssue(iss Issue) []string {
	return []string{
		strconv.Itoa(iss.Number),
		iss.ExpertAssessment,
		strconv.FormatBool(iss.DisagreeFlag),
	}
}
