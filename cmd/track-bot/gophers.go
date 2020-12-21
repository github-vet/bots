package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"
)

const gopherNumFields int = 3

// Gopher represents a GitHub user not marked as an 'expert' for the purpose of crowdsourcing.
type Gopher struct {
	Username      string
	Disagreements int
	Assessments   int
}

// AnonymousScore is the score of a gopher who has never had one of their votes assessed.
const AnonymousScore float32 = 0.25

// Score computes a value in (0, 1) reflecting how frequently this Gopher has agreed with expert opinion.
func (u Gopher) Score() float32 {
	return float32(1+u.Assessments-u.Disagreements) / float32(1+u.Assessments)
}

// Score retrieves the score of the Gopher from the map. Unknown gophers receive the default score.
func Score(gophers map[string]*Gopher, name string) float32 {
	if goph, ok := gophers[name]; ok {
		return goph.Score()
	}
	return AnonymousScore
}

func gopherFromCsvLine(line []string) (Gopher, error) {
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

func csvLineFromGopher(exp Gopher) []string {
	return []string{
		exp.Username,
		strconv.Itoa(exp.Disagreements),
		strconv.Itoa(exp.Assessments),
	}
}

// ReadGophersFile opens the file at the provided path and reads it into a
// map of Gophers, keyed by username.
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
		if len(record) != gopherNumFields {
			log.Printf("malformed line in issues list %s line %d, expected %d fields, found %d", path, lineNum, issueNumFields, len(record))
			continue
		}
		gopher, err := gopherFromCsvLine(record)
		if err != nil {
			log.Printf("malformed line in issues list %s line %d: %v", path, lineNum, err)
		}
		result[gopher.Username] = &gopher
	}
	return result, nil
}

// WriteGophersFile writes the provided map of gophers to the given file, truncating
// its contents.
func WriteGophersFile(path string, gophers map[string]*Gopher) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	for _, goph := range gophers {
		writer.Write(csvLineFromGopher(*goph))
	}
	writer.Flush()
	return nil
}
