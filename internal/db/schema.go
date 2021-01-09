package db

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// BootstrapDB attempts to execute all files ending in .sql from the provided directory against the
// provided database, in alphabetical order by filename. If no files are found, an error is returned.
func BootstrapDB(schemaFolder string, DB *sql.DB) error {
	files, err := ioutil.ReadDir(schemaFolder)
	if err != nil {
		return err
	}
	foundSQLFile := false
	for _, finfo := range files {
		if finfo.IsDir() {
			continue
		}
		if !strings.HasSuffix(finfo.Name(), ".sql") {
			continue
		}
		foundSQLFile = true

		script, err := ioutil.ReadFile(filepath.Join(schemaFolder, finfo.Name()))
		if err != nil {
			return err
		}
		_, err = DB.Exec(string(script))
		if err != nil {
			log.Printf("could not execute bootstrap script %s: %v", finfo.Name(), err)
			return err
		}
		log.Printf("executed bootstrap script %s", finfo.Name())
	}
	if !foundSQLFile {
		return fmt.Errorf("could not find any *.sql files in schema folder %s", schemaFolder)
	}
	return nil
}

// SeedRepositories loads the repositories from the provided CSV file into the database only
// if there are not any repository records already present in the database.
func SeedRepositories(repoCsv string, DB *sql.DB) error {
	count, err := RepositoryDAO.CountAll(context.Background(), DB)
	if err != nil {
		return err
	}
	if count != 0 {
		return nil
	}

	file, err := os.OpenFile(repoCsv, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = 2

	stmt, err := DB.Prepare("INSERT INTO repositories(github_owner, github_repo, state) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("could not prepare statement: %w", err)
	}
	defer stmt.Close()

	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("could not open transaction: %w", err)
	}
	defer tx.Commit()

	var line int
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error at line %d; could not seed all repositories into database, a partial load may have occurred: %w", line, err)
		}

		tx.Stmt(stmt).Exec(record[0], record[1], RepoStateFresh)
		if err != nil {
			return fmt.Errorf("could not write record %d to database: %w", line, err)
		}
		line++
	}

	// cleanup the seed file only if seeding was successful.
	err = os.Remove(repoCsv)
	if err != nil {
		log.Fatalf("could not clean up seed file %s: %v", repoCsv, err)
	}
	return nil
}
