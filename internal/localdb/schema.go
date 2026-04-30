package localdb

const schemaSQL = `
CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    signal_type     TEXT NOT NULL,
    content         TEXT NOT NULL,
    source_type     TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    repo_name       TEXT,
    branch_name     TEXT,
    file_path       TEXT,
    freeform_context TEXT,
    structured_data TEXT,
    author_type     TEXT DEFAULT 'human',
    author_label    TEXT,
    synced          INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS commits (
    sha             TEXT PRIMARY KEY,
    repo_path       TEXT NOT NULL,
    repo_name       TEXT,
    branch          TEXT,
    author_name     TEXT,
    author_email    TEXT,
    message         TEXT NOT NULL,
    committed_at    TEXT NOT NULL,
    files_changed   INTEGER,
    insertions      INTEGER,
    deletions       INTEGER,
    is_merge        INTEGER DEFAULT 0,
    indexed_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS commit_files (
    commit_sha      TEXT NOT NULL REFERENCES commits(sha),
    file_path       TEXT NOT NULL,
    change_type     TEXT,
    old_path        TEXT,
    PRIMARY KEY (commit_sha, file_path)
);

CREATE TABLE IF NOT EXISTS links (
    id              TEXT PRIMARY KEY,
    source_id       TEXT NOT NULL,
    target_id       TEXT NOT NULL,
    link_type       TEXT NOT NULL,
    confidence      REAL DEFAULT 1.0,
    reason          TEXT,
    created_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backlog_items (
    id              TEXT PRIMARY KEY,
    short_id        INTEGER,
    title           TEXT NOT NULL,
    summary         TEXT,
    body            TEXT,
    status          TEXT DEFAULT 'open',
    category        TEXT DEFAULT 'task',
    urgency         TEXT DEFAULT 'medium',
    importance      TEXT DEFAULT 'medium',
    repo_name       TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    synced          INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS repos (
    path            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    last_indexed_sha TEXT,
    last_indexed_at TEXT,
    is_squash_heavy INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS identifiers (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    value           TEXT NOT NULL,
    provider        TEXT,
    url             TEXT,
    first_seen_at   TEXT NOT NULL,
    UNIQUE(kind, value)
);

CREATE INDEX IF NOT EXISTS idx_events_signal_type ON events(signal_type);
CREATE INDEX IF NOT EXISTS idx_events_repo ON events(repo_name);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_commits_repo ON commits(repo_path);
CREATE INDEX IF NOT EXISTS idx_commits_date ON commits(committed_at DESC);
CREATE INDEX IF NOT EXISTS idx_commit_files_path ON commit_files(file_path);
CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_id);
CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_id);
CREATE INDEX IF NOT EXISTS idx_identifiers_kind ON identifiers(kind);
CREATE INDEX IF NOT EXISTS idx_identifiers_value ON identifiers(value);
`
