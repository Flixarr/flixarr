-- Plex Watchlist requests: each person's plex.tv token (encrypted, from sign-in), their opt-in, and the
-- watchlist titles already handled so each is only requested once.
ALTER TABLE users ADD COLUMN plex_token TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN watchlist INTEGER NOT NULL DEFAULT 0;
CREATE TABLE watchlist_seen (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  result TEXT NOT NULL,     -- requested | in-library | already-requested | failed: …
  seen_at TEXT NOT NULL,
  PRIMARY KEY (user_id, type, tmdb_id)
);
