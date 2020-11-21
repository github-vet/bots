package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"
)

const ExpertNumFields int = 2

type Expert struct {
	Username        string
	AssessmentCount int
}

func ExpertFromCsvLine(line []string) (Expert, error) {
	assessCount, err := strconv.ParseInt(line[1], 10, 32)
	if err != nil {
		return Expert{}, err
	}
	return Expert{
		Username:        line[0],
		AssessmentCount: int(assessCount),
	}, nil
}

func CsvLineFromExpert(exp Expert) []string {
	return []string{
		exp.Username,
		strconv.Itoa(exp.AssessmentCount),
	}
}
func ReadExpertsFile(path string) (map[string]*Expert, error) {
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
		if len(record) != IssueNumFields {
			log.Printf("malformed line in issues list %s line %d, expected %d fields, found %d", path, lineNum, IssueNumFields, len(record))
			continue
		}
		expert, err := ExpertFromCsvLine(record)
		if err != nil {
			log.Printf("malformed line in issues list %s line %d: %v", path, lineNum, err)
		}
		result[expert.Username] = &expert
	}
	return result, nil
}

func WriteExpertsFile(path string, experts map[string]*Expert) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	for _, exp := range experts {
		writer.Write(CsvLineFromExpert(*exp))
	}
	writer.Flush()
	return nil
}
