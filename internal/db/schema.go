package db

import (
	"database/sql"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
)

// BootstrapDB attempts to execute all files ending in .sql from the provided directory against the
// provided database, in alphabetical order by filename.
func BootstrapDB(schemaFolder string, DB *sql.DB) error {
	files, err := ioutil.ReadDir(schemaFolder)
	if err != nil {
		return err
	}
	for _, finfo := range files {
		if finfo.IsDir() {
			continue
		}
		if !strings.HasSuffix(finfo.Name(), ".sql") {
			continue
		}

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
	return nil
}
