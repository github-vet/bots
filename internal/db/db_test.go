package db_test

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/github-vet/bots/internal/db"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
)

var DB *sql.DB

func TestMain(m *testing.M) {
	dbPath := fmt.Sprintf("%s/test.db", os.TempDir())

	// delete any existing database
	err := os.Truncate(dbPath, 0)

	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("could not truncate database file %s: %v", dbPath, err)
	}

	// open DB and load schema
	DB, err = sql.Open("sqlite3", dbPath)
	defer DB.Close()

	loadSchema(DB, "schema/v1.sql")
	if err != nil {
		log.Fatalf("could not open database %s: %v", dbPath, err)
	}

	m.Run()

	os.Remove(dbPath)
}

// loadSchema opens the sql script at path and executes it on the provided database.
func loadSchema(db *sql.DB, path string) {
	schemaBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("could not read schema from %s: %v", path, err)
	}

	_, err = db.Exec(string(schemaBytes))
	if err != nil {
		log.Fatalf("could not execute schema from %s: %v", path, err)
	}
}

func TestIssueDAO(t *testing.T) {
	ctx := context.Background()

	count, err := db.IssueDAO.Upsert(ctx, DB, db.Issue{
		GithubOwner: "test",
		GithubRepo:  "123",
		GithubID:    2,
	})
	assert.EqualValues(t, 1, count)
	assert.NoError(t, err)

	issue, err := db.IssueDAO.FindByCoordinates(ctx, DB, "test", "123", 2)
	assert.NoError(t, err)
	assert.Equal(t, "test", issue.GithubOwner)
	assert.Equal(t, "123", issue.GithubRepo)
	assert.Equal(t, 2, issue.GithubID)
	assert.Zero(t, issue.ExpertAssessment)
	assert.False(t, issue.ExpertsDisagree())

	issue.ExpertAssessment = "confused"
	issue.SetExpertsDisagree(true)
	count, err = db.IssueDAO.Upsert(ctx, DB, issue)

	issue, err = db.IssueDAO.FindByCoordinates(ctx, DB, "test", "123", 2)
	assert.NoError(t, err)
	assert.Equal(t, "confused", issue.ExpertAssessment)
	assert.True(t, issue.ExpertsDisagree())
}

func TestRepositoryDAO(t *testing.T) {
	ctx := context.Background()

	count, err := db.RepositoryDAO.Upsert(ctx, DB, db.NewRepository("test", "123"))
	assert.EqualValues(t, 1, count)
	assert.NoError(t, err)

	repo, err := db.RepositoryDAO.FindByID(ctx, DB, "test", "123")
	assert.NoError(t, err)
	assert.Equal(t, "test", repo.GithubOwner)
	assert.Equal(t, "123", repo.GithubRepo)
	assert.Equal(t, db.RepoStateFresh, repo.State)

	repo.State = db.RepoStateVisited
	count, err = db.RepositoryDAO.Upsert(ctx, DB, repo)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, count)

	repo, err = db.RepositoryDAO.FindByID(ctx, DB, "test", "123")
	assert.NoError(t, err)
	assert.Equal(t, db.RepoStateVisited, repo.State)

	count, err = db.RepositoryDAO.Upsert(ctx, DB, db.NewRepository("test", "456"))
	assert.EqualValues(t, 1, count)
	assert.NoError(t, err)

	visitedRepos, err := db.RepositoryDAO.ListByState(ctx, DB, db.RepoStateVisited)
	assert.NoError(t, err)
	freshRepos, err := db.RepositoryDAO.ListByState(ctx, DB, db.RepoStateFresh)
	assert.NoError(t, err)

	assert.Len(t, visitedRepos, 1)
	assert.Len(t, freshRepos, 1)

	assert.Equal(t, visitedRepos[0].GithubRepo, "123")
	assert.Equal(t, freshRepos[0].GithubRepo, "456")
}

func TestGopherDAO(t *testing.T) {
	ctx := context.Background()

	count, err := db.GopherDAO.Upsert(ctx, DB, db.Gopher{Username: "kalexmills"})
	assert.EqualValues(t, 1, count)
	assert.NoError(t, err)

	g, err := db.GopherDAO.FindByUsername(ctx, DB, "kalexmills")
	assert.NoError(t, err)
	assert.Equal(t, "kalexmills", g.Username)
	assert.Equal(t, 0, g.AssessmentCount)
	assert.Equal(t, 0, g.DisagreementCount)

	g.AssessmentCount = 2
	g.DisagreementCount = 5
	count, err = db.GopherDAO.Upsert(ctx, DB, g)
	assert.EqualValues(t, 1, count)
	assert.NoError(t, err)

	g, err = db.GopherDAO.FindByUsername(ctx, DB, "kalexmills")
	assert.NoError(t, err)
	assert.EqualValues(t, 2, g.AssessmentCount)
	assert.EqualValues(t, 5, g.DisagreementCount)
}

func TestFindingDAO(t *testing.T) {
	ctx := context.Background()

	hash := md5.Sum([]byte("quote"))

	count, err := db.FindingDAO.Create(ctx, DB, db.Finding{
		GithubOwner:  "owner",
		GithubRepo:   "repo",
		Filepath:     "filepath",
		RootCommitID: "rootCommit",
		Quote:        "quote",
		QuoteMD5Sum:  hash[:],
		StartLine:    2,
		EndLine:      7,
		Message:      "message",
		ExtraInfo:    "extra",
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	id, err := db.LastInsertID(DB)
	assert.NoError(t, err)
	assert.Equal(t, 1, id)

	count, err = db.FindingDAO.Create(ctx, DB, db.Finding{
		GithubOwner:  "owner",
		GithubRepo:   "repo",
		Filepath:     "filepath",
		RootCommitID: "rootCommit",
		Quote:        "quote",
		QuoteMD5Sum:  hash[:],
		StartLine:    2,
		EndLine:      7,
		Message:      "message",
		ExtraInfo:    "extra",
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	id, err = db.LastInsertID(DB)
	assert.NoError(t, err)
	assert.Equal(t, 2, id)

	f, err := db.FindingDAO.FindByID(ctx, DB, 1)
	assert.NoError(t, err)
	assert.Equal(t, "owner", f.GithubOwner)
	assert.Equal(t, "repo", f.GithubRepo)
	assert.Equal(t, "filepath", f.Filepath)
	assert.Equal(t, "rootCommit", f.RootCommitID)
	assert.Equal(t, "quote", f.Quote)
	assert.Equal(t, db.Md5Sum(hash[:]), f.QuoteMD5Sum)
	assert.Equal(t, 2, f.StartLine)
	assert.Equal(t, 7, f.EndLine)
	assert.Equal(t, "message", f.Message)
	assert.Equal(t, "extra", f.ExtraInfo)
}
