-- Requests for movies and series. One open request per title; a declined or failed one can be requested again.
CREATE TABLE requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL CHECK (type IN ('movie', 'series')),
  tmdb_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  year TEXT NOT NULL DEFAULT '',
  poster TEXT NOT NULL DEFAULT '',
  backdrop TEXT NOT NULL DEFAULT '',
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL,            -- pending | approved | declined | sent | downloading | available | failed
  server_id INTEGER,               -- the Radarr/Sonarr it went to
  arr_id INTEGER NOT NULL DEFAULT 0, -- the movie/series ID in Radarr/Sonarr
  profile_id INTEGER NOT NULL DEFAULT 0,
  profile_name TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'all', -- series: all | seasons
  seasons TEXT NOT NULL DEFAULT '[]', -- series: requested season numbers (JSON)
  progress INTEGER NOT NULL DEFAULT 0, -- percent, while downloading
  error TEXT NOT NULL DEFAULT '',
  anime INTEGER NOT NULL DEFAULT 0, -- series: send as Sonarr's anime type
  decided_by INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX requests_title ON requests(type, tmdb_id);
CREATE INDEX requests_user ON requests(user_id);
CREATE INDEX requests_status ON requests(status);
