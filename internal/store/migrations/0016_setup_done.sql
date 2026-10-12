-- First-run setup now continues after sign-in (Plex server, libraries, Radarr/Sonarr). Installs that
-- already have people are past it.
INSERT INTO settings (key, value) SELECT 'setup_done', 'true' WHERE EXISTS (SELECT 1 FROM users)
	ON CONFLICT(key) DO NOTHING;
