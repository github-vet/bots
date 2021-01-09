package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jonbodner/proteus"
)

type Finding struct {
	ID           int64  `prof:"id"`
	GithubOwner  string `prof:"github_owner"`
	GithubRepo   string `prof:"github_repo"`
	Filepath     string `prof:"filepath"`
	RootCommitID string `prof:"root_commit_id"`
	Quote        string `prof:"quote"`
	QuoteMD5Sum  Md5Sum `prof:"quote_md5sum"`
	StartLine    int    `prof:"start_line"`
	EndLine      int    `prof:"end_line"`
	Message      string `prof:"message"`
	ExtraInfo    string `prof:"extra_info"`
}

type Md5Sum []byte

type FindingDaoImpl struct {
	Create        func(ctx context.Context, e proteus.ContextExecutor, f Finding) (int64, error) `proq:"q:create" prop:"f"`
	FindByID      func(ctx context.Context, q proteus.ContextQuerier, id int64) (Finding, error) `proq:"q:findById" prop:"id"`
	ListChecksums func(ctx context.Context, q proteus.ContextQuerier) ([]Md5Sum, error)          `proq:"q:listChecksums"`
}

var FindingDAO FindingDaoImpl

func init() {
	m := proteus.MapMapper{
		"create": `INSERT INTO findings (github_repo, github_owner, quote_md5sum, filepath, root_commit_id, quote, quote_md5sum, start_line, end_line, message, extra_info) 
							 VALUES (:f.GithubRepo:, :f.GithubOwner:, :f.QuoteMD5Sum:, :f.Filepath:, :f.RootCommitID:, :f.Quote:, :f.QuoteMD5Sum:, :f.StartLine:, :f.EndLine:, :f.Message:, :f.ExtraInfo:)`,

		"findById":      `SELECT * FROM findings WHERE id = :id:`,
		"listChecksums": `SELECT quote_md5sum FROM findings`,
		"lastUpdateId":  `SELECT last_update_id()`,
	}
	err := proteus.ShouldBuild(context.Background(), &FindingDAO, proteus.Sqlite, m)
	if err != nil {
		panic(err)
	}
}

// LastInsertID retrieves the last insert ID. Its use is prone to race-conditions in case multiple
// queries are being executed simultaneously.
func LastInsertID(db *sql.DB) (int, error) {
	rows, err := db.Query("SELECT last_insert_rowid()")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var result int
		rows.Scan(&result)
		return result, nil
	}
	return 0, errors.New("query resulted in zero rows")
}
