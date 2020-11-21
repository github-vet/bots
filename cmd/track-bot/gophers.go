package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"
)

const GopherNumFields int = 3

type Gopher struct {
	Username      string
	Disagreements int
	Assessments   int
}

// AnonymousScore is the score of a gopher who has never had one of their votes assessed.
const AnonymousScore float32 = 0.25

func (u Gopher) Score() float32 {
	return 1 - float32(1+u.Disagreements)/float32(1+u.Assessments)
}

// Score retrieves the score of the Gopher from the map. Unknown gophers receive the default score.
func Score(gophers map[string]*Gopher, name string) float32 {
	if goph, ok := gophers[name]; ok {
		return goph.Score()
	}
	return AnonymousScore
}

func GopherFromCsvLine(line []string) (Gopher, error) {
	disagreeCount, err := strconv.ParseInt(line[1], 10, 32)
	if err != nil {
		return Gopher{}, err
	}
	assessCount, err := strconv.ParseInt(line[2], 10, 32)
	if err != nil {
		return Gopher{}, err
	}
	return Gopher{
		Username:      line[0],
		Disagreements: int(disagreeCount),
		Assessments:   int(assessCount),
	}, nil
}

func CsvLineFromGopher(exp Gopher) []string {
	return []string{
		exp.Username,
		strconv.Itoa(exp.Disagreements),
		strconv.Itoa(exp.Assessments),
	}
}

func ReadGophersFile(path string) (map[string]*Gopher, error) {
	result := make(map[string]*Gopher)
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
		if len(record) != GopherNumFields {
			log.Printf("malformed line in issues list %s line %d, expected %d fields, found %d", path, lineNum, IssueNumFields, len(record))
			continue
		}
		gopher, err := GopherFromCsvLine(record)
		if err != nil {
			log.Printf("malformed line in issues list %s line %d: %v", path, lineNum, err)
		}
		result[gopher.Username] = &gopher
	}
	return result, nil
}

func WriteGophersFile(path string, gophers map[string]*Gopher) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	for _, goph := range gophers {
		writer.Write(CsvLineFromGopher(*goph))
	}
	writer.Flush()
	return nil
}
