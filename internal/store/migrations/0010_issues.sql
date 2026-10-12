-- Problems reported on titles in the library, and their comment threads.
CREATE TABLE issues (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,                 -- movie | series
  tmdb_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  year TEXT NOT NULL DEFAULT '',
  poster TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,                 -- Video | Audio | Subtitles | Other
  sub TEXT NOT NULL DEFAULT '',       -- e.g. Too quiet
  season INTEGER NOT NULL DEFAULT 0,  -- 0 = all seasons (series only)
  episode INTEGER NOT NULL DEFAULT 0, -- 0 = all episodes
  note TEXT NOT NULL DEFAULT '',
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'open', -- open | resolved
  resolved_by INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX issues_status ON issues(status);
CREATE INDEX issues_user ON issues(user_id);
CREATE TABLE issue_comments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX issue_comments_issue ON issue_comments(issue_id);
