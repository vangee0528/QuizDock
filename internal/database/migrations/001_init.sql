PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS question_banks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT '',
    exam TEXT NOT NULL DEFAULT '',
    level TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    license TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    manifest_json TEXT NOT NULL,
    archive_hash TEXT NOT NULL,
    question_count INTEGER NOT NULL DEFAULT 0,
    installed INTEGER NOT NULL DEFAULT 1,
    enabled INTEGER NOT NULL DEFAULT 1,
    imported_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS questions (
    uid TEXT PRIMARY KEY,
    bank_id TEXT NOT NULL REFERENCES question_banks(id),
    question_id TEXT NOT NULL,
    title TEXT NOT NULL,
    chapter TEXT NOT NULL DEFAULT '',
    topic TEXT NOT NULL DEFAULT '',
    question_type TEXT NOT NULL,
    order_index INTEGER NOT NULL DEFAULT 0,
    exam TEXT NOT NULL DEFAULT '',
    difficulty TEXT NOT NULL DEFAULT '',
    stem_md TEXT NOT NULL,
    explanation_md TEXT NOT NULL DEFAULT '',
    raw_markdown TEXT NOT NULL,
    ready INTEGER NOT NULL DEFAULT 0,
    active INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL,
    UNIQUE(bank_id, question_id)
);

CREATE INDEX IF NOT EXISTS idx_questions_queue
ON questions(active, ready, bank_id, chapter, exam, order_index, question_id);

CREATE TABLE IF NOT EXISTS question_parts (
    question_uid TEXT NOT NULL REFERENCES questions(uid) ON DELETE CASCADE,
    part_index INTEGER NOT NULL,
    label TEXT NOT NULL,
    prompt_md TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(question_uid, part_index)
);

CREATE TABLE IF NOT EXISTS question_options (
    question_uid TEXT NOT NULL,
    part_index INTEGER NOT NULL,
    option_order INTEGER NOT NULL,
    label TEXT NOT NULL,
    body_md TEXT NOT NULL,
    is_correct INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(question_uid, part_index, label),
    FOREIGN KEY(question_uid, part_index)
      REFERENCES question_parts(question_uid, part_index) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS question_tags (
    question_uid TEXT NOT NULL REFERENCES questions(uid) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY(question_uid, tag)
);

CREATE INDEX IF NOT EXISTS idx_question_tags_tag ON question_tags(tag, question_uid);

CREATE TABLE IF NOT EXISTS assets (
    bank_id TEXT NOT NULL REFERENCES question_banks(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    data BLOB NOT NULL,
    PRIMARY KEY(bank_id, path)
);

CREATE INDEX IF NOT EXISTS idx_assets_hash ON assets(sha256);

CREATE TABLE IF NOT EXISTS attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    question_uid TEXT NOT NULL REFERENCES questions(uid),
    answers_json TEXT NOT NULL,
    correct INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_attempts_question ON attempts(question_uid, id DESC);

CREATE TABLE IF NOT EXISTS study_progress (
    scope_key TEXT PRIMARY KEY,
    mode TEXT NOT NULL,
    filters_json TEXT NOT NULL,
    current_uid TEXT NOT NULL,
    current_index INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS question_state (
    question_uid TEXT PRIMARY KEY REFERENCES questions(uid),
    starred INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS question_mastery (
    question_uid TEXT PRIMARY KEY REFERENCES questions(uid),
    wrong_count INTEGER NOT NULL DEFAULT 0,
    correct_count INTEGER NOT NULL DEFAULT 0,
    consecutive_correct INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'learning',
    due_date TEXT NOT NULL,
    last_wrong_at TEXT,
    last_correct_at TEXT,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_mastery_due ON question_mastery(status, due_date);

CREATE TABLE IF NOT EXISTS app_settings (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    track_wrong INTEGER NOT NULL DEFAULT 1,
    daily_target INTEGER NOT NULL DEFAULT 20,
    required_streak INTEGER NOT NULL DEFAULT 3,
    ai_url TEXT NOT NULL DEFAULT 'https://chat.deepseek.com/',
    updated_at TEXT NOT NULL
);

INSERT OR IGNORE INTO app_settings(
    id, track_wrong, daily_target, required_streak, ai_url, updated_at
) VALUES(1, 1, 20, 3, 'https://chat.deepseek.com/', CURRENT_TIMESTAMP);
