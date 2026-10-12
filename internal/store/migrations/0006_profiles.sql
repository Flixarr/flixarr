-- Flixarr quality profiles added to a Radarr/Sonarr server. The stored IDs are how Flixarr finds its
-- profiles and custom formats again (names can be changed in Radarr/Sonarr).
CREATE TABLE profile_links (
  server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  key TEXT NOT NULL,              -- Flixarr profile id, e.g. "standard"
  arr_id INTEGER NOT NULL,        -- the profile's ID in Radarr/Sonarr
  rev TEXT NOT NULL,              -- the Flixarr revision last pushed
  pushed_hash TEXT NOT NULL,      -- fingerprint of the profile as pushed, to spot edits made in Radarr/Sonarr
  state TEXT NOT NULL DEFAULT 'ok', -- ok | customized | missing
  checked_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (server_id, key)
);
CREATE TABLE format_links (
  server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  name TEXT NOT NULL,             -- Flixarr's name for the format
  arr_id INTEGER NOT NULL,
  owned INTEGER NOT NULL,         -- 1 = Flixarr created it and may update it; 0 = it was already there (never changed)
  spec_hash TEXT NOT NULL,
  PRIMARY KEY (server_id, name)
);
