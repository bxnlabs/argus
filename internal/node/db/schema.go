package db

const schema = `
-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  tmux_name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  working_directory TEXT NOT NULL DEFAULT '~',
  provider_session_id TEXT,
  model TEXT DEFAULT 'sonnet',
  system_prompt TEXT,
  provider_type TEXT NOT NULL DEFAULT 'claude',
  auto_approve INTEGER NOT NULL DEFAULT 0,
  worktree_branch TEXT,
  git_parent_dir TEXT,
  git_remote_url TEXT,
  profile TEXT,
  branch_created INTEGER NOT NULL DEFAULT 0,
  unread_since TEXT,
  last_viewed_at TEXT
);

-- Notifications tracking
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  sent_at TEXT NOT NULL,
  FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notifications_session_sent_at
  ON notifications(session_id, sent_at);

-- Migrations tracking
CREATE TABLE IF NOT EXISTS _migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

`
