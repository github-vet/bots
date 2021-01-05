CREATE TABLE gophers (
  username           TEXT PRIMARY KEY,
  disagreement_count INTEGER DEFAULT 0 NOT NULL,
  assessment_count   INTEGER DEFAULT 0 NOT NULL
);

CREATE TABLE experts (
  username TEXT PRIMARY KEY,
  assessment_count INTEGER DEFAULT 0 NOT NULL
);

CREATE TABLE repositories (
  github_owner TEXT NOT NULL,
  github_repo  TEXT NOT NULL,
  state        TEXT CHECK (state in ('F', 'V', 'E') )  NOT NULL DEFAULT 'F',
                -- 'F' = "fresh"; 'V' = "visited"; 'E' = "errored"
  PRIMARY KEY (github_repo, github_owner)
);

CREATE TABLE findings (
  id             INTEGER PRIMARY KEY,
  github_owner   TEXT NOT NULL,
  github_repo    TEXT NOT NULL,
  filepath       TEXT NOT NULL,
  root_commit_id TEXT NOT NULL,
  quote          TEXT NOT NULL,
  quote_md5sum   BLOB NOT NULL,   -- md5 sum of the quote
  start_line     INTEGER NOT NULL,
  end_line       INTEGER NOT NULL,
  message        TEXT NOT NULL,
  extra_info     TEXT NOT NULL
);

CREATE TABLE issues (
  finding_id          INTEGER PRIMARY KEY,
  github_owner        TEXT NOT NULL,
  github_repo         TEXT NOT NULL,
  github_id           INTEGER NOT NULL,
  expert_assessment   TEXT,
  expert_disagreement INTEGER CHECK (expert_disagreement in (0, 1))  DEFAULT 0  NOT NULL,
  FOREIGN KEY(finding_id) REFERENCES findings(id)
  UNIQUE (github_owner, github_repo, github_id)
);
