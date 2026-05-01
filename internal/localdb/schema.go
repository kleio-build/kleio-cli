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
CREATE UNIQUE INDEX IF NOT EXISTS idx_links_dedup ON links(source_id, target_id, link_type);
CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_id);
CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_id);
-- Cluster reconstruction filters by (link_type, target_id) to find every
-- member of a cluster given its anchor signal id; pipeline persistence
-- inserts via these columns at high volume.
CREATE INDEX IF NOT EXISTS idx_links_type_target ON links(link_type, target_id);
CREATE INDEX IF NOT EXISTS idx_links_type_source ON links(link_type, source_id);
CREATE INDEX IF NOT EXISTS idx_identifiers_kind ON identifiers(kind);
CREATE INDEX IF NOT EXISTS idx_identifiers_value ON identifiers(value);

CREATE TABLE IF NOT EXISTS entities (
    id              TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    label           TEXT NOT NULL,
    normalized_label TEXT NOT NULL,
    repo_name       TEXT,
    first_seen_at   TEXT NOT NULL,
    last_seen_at    TEXT NOT NULL,
    mention_count   INTEGER DEFAULT 1,
    confidence      REAL DEFAULT 0.5
);
CREATE INDEX IF NOT EXISTS idx_entities_kind ON entities(kind);
CREATE INDEX IF NOT EXISTS idx_entities_normalized ON entities(normalized_label);

CREATE TABLE IF NOT EXISTS entity_aliases (
    entity_id       TEXT NOT NULL REFERENCES entities(id),
    alias           TEXT NOT NULL,
    source          TEXT NOT NULL,
    confidence      REAL NOT NULL,
    created_at      TEXT NOT NULL,
    UNIQUE(entity_id, alias)
);
CREATE INDEX IF NOT EXISTS idx_entity_aliases_alias ON entity_aliases(alias);

CREATE TABLE IF NOT EXISTS entity_mentions (
    id              TEXT PRIMARY KEY,
    entity_id       TEXT NOT NULL REFERENCES entities(id),
    evidence_type   TEXT NOT NULL,
    evidence_id     TEXT NOT NULL,
    mention_context TEXT,
    confidence      REAL NOT NULL,
    created_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_entity_mentions_entity ON entity_mentions(entity_id);
CREATE INDEX IF NOT EXISTS idx_entity_mentions_evidence ON entity_mentions(evidence_id);

CREATE TABLE IF NOT EXISTS code_symbols (
    id              TEXT PRIMARY KEY,
    repo_name       TEXT NOT NULL,
    file_path       TEXT NOT NULL,
    symbol_name     TEXT NOT NULL,
    symbol_kind     TEXT NOT NULL,
    start_line      INTEGER,
    end_line        INTEGER,
    language        TEXT NOT NULL,
    last_commit_sha TEXT
);
CREATE INDEX IF NOT EXISTS idx_code_symbols_repo_file ON code_symbols(repo_name, file_path);

CREATE TABLE IF NOT EXISTS commit_symbol_changes (
    commit_sha      TEXT NOT NULL,
    symbol_id       TEXT NOT NULL REFERENCES code_symbols(id),
    change_kind     TEXT NOT NULL,
    lines_added     INTEGER,
    lines_deleted   INTEGER,
    PRIMARY KEY (commit_sha, symbol_id)
);

CREATE TABLE IF NOT EXISTS derived_edges (
    id                TEXT PRIMARY KEY,
    source_entity_id  TEXT NOT NULL,
    predicate         TEXT NOT NULL,
    target_entity_id  TEXT NOT NULL,
    rule_name         TEXT NOT NULL,
    confidence        REAL NOT NULL,
    evidence_count    INTEGER DEFAULT 1,
    generated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_derived_edges_source ON derived_edges(source_entity_id);
CREATE INDEX IF NOT EXISTS idx_derived_edges_target ON derived_edges(target_entity_id);
CREATE INDEX IF NOT EXISTS idx_derived_edges_predicate ON derived_edges(predicate);

CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
    event_id UNINDEXED, content, freeform_context
);
`
