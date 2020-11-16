package main

import (
	"encoding/csv"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	file, err := os.OpenFile("go-repos.csv", os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	out, err := os.OpenFile("out.csv", os.O_CREATE|os.O_APPEND, 0666)
	defer file.Close()
	if err != nil {
		log.Fatalf("%v", err)
	}
	writer := csv.NewWriter(out)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) != 4 {
			continue
		}
		gitURL := record[1]
		fullname := strings.Replace(strings.Replace(gitURL, "git://github.com/", "", 1), ".git", "", 1)

		record = append(record, fullname)

		writer.Write(record)
	}

}
