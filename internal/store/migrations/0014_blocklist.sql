-- Titles admins blocked from being requested.
CREATE TABLE blocklist (
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  year TEXT NOT NULL DEFAULT '',
  poster TEXT NOT NULL DEFAULT '',
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (type, tmdb_id)
);
