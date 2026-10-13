-- Maintainerr: titles in its leaving collections (rows stay a while after they go, so a removal can be
-- told apart from any other), titles Flixarr kept out of its rules, and people asking to keep a title.
CREATE TABLE leaving (
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  plex_id TEXT NOT NULL,
  collection_id INTEGER NOT NULL,
  collection TEXT NOT NULL,
  leave_at TEXT NOT NULL,
  seen_at TEXT NOT NULL,
  PRIMARY KEY (type, tmdb_id)
);
CREATE TABLE keeps (
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  plex_id TEXT NOT NULL,
  reason TEXT NOT NULL,          -- kept (by an admin) or fresh (a new request, for a while)
  until_at TEXT NOT NULL DEFAULT '', -- fresh keeps end then
  by_user INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY (type, tmdb_id)
);
CREATE TABLE keep_asks (
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  PRIMARY KEY (type, tmdb_id, user_id)
);
CREATE TABLE leaving_told (
  request_id INTEGER PRIMARY KEY,
  leave_at TEXT NOT NULL
);
