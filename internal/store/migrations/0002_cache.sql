CREATE TABLE cache (
  key TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  status INTEGER NOT NULL,
  body BLOB NOT NULL,
  fetched_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX cache_kind ON cache(kind);
CREATE INDEX cache_fetched ON cache(fetched_at);
