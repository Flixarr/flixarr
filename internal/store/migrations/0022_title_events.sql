-- A title's own history (the admin panel's History), beside its requests' and issues' timelines:
-- added to Plex, Redownload, deleted, data cleared. Kept when a title's data is cleared.
CREATE TABLE title_events (
  id INTEGER PRIMARY KEY,
  type TEXT NOT NULL,
  tmdb_id INTEGER NOT NULL,
  kind TEXT NOT NULL,          -- plex, redownload, deleted, cleared
  note TEXT NOT NULL DEFAULT '',
  user_id INTEGER,             -- who did it (none for Plex)
  at TEXT NOT NULL
);
CREATE INDEX title_events_title ON title_events (type, tmdb_id);
-- Titles already in Plex: their arrival, from Plex's own added date.
INSERT INTO title_events (type, tmdb_id, kind, at)
  SELECT type, tmdb_id, 'plex', strftime('%Y-%m-%dT%H:%M:%SZ', added_at, 'unixepoch') FROM plex_items WHERE added_at > 0;
