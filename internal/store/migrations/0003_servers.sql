-- Radarr and Sonarr connections. Stored as a list so more than one server per kind can be added
-- later; the Settings screen manages one of each for now. Requests will record their server_id.
CREATE TABLE servers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL CHECK (kind IN ('radarr', 'sonarr')),
  name TEXT NOT NULL,
  url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  root_folder TEXT NOT NULL DEFAULT '',
  quality_profile_id INTEGER NOT NULL DEFAULT 0,
  quality_profile_name TEXT NOT NULL DEFAULT '',
  season_folders INTEGER NOT NULL DEFAULT 1,
  search_on_add INTEGER NOT NULL DEFAULT 1,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX servers_kind ON servers(kind);
