package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"
)

const expertNumFields int = 2

// Expert describes a GitHub user marked as an 'expert' for the purpose of crowd sourcing.
type Expert struct {
	Username        string
	AssessmentCount int
}

func expertFromCsvLine(line []string) (Expert, error) {
	assessCount, err := strconv.ParseInt(line[1], 10, 32)
	if err != nil {
		return Expert{}, err
	}
	return Expert{
		Username:        line[0],
		AssessmentCount: int(assessCount),
	}, nil
}

func csvLineFromExpert(exp Expert) []string {
	return []string{
		exp.Username,
		strconv.Itoa(exp.AssessmentCount),
	}
}

// ReadExpertsFile opens the provided file and parses the contents into a map of
// Experts, keyed by username.
func ReadExpertsFile(path string) (map[string]*Expert, error) {
	// TODO: mayhaps too much duplication
	result := make(map[string]*Expert)
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
		if len(record) != expertNumFields {
			log.Printf("malformed line in issues list %s line %d, expected %d fields, found %d", path, lineNum, issueNumFields, len(record))
			continue
		}
		expert, err := expertFromCsvLine(record)
		if err != nil {
			log.Printf("malformed line in issues list %s line %d: %v", path, lineNum, err)
		}
		result[expert.Username] = &expert
	}
	return result, nil
}

// WriteExpertsFile writes the provided list of experts to the provided path, truncating
// whatever file may exist.
func WriteExpertsFile(path string, experts map[string]*Expert) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	for _, exp := range experts {
		writer.Write(csvLineFromExpert(*exp))
	}
	writer.Flush()
	return nil
}
