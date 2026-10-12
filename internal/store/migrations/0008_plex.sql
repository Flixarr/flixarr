-- Titles in the Plex server's movie and TV libraries, by TMDB ID.
CREATE TABLE plex_items (
	type       TEXT NOT NULL,              -- movie | series
	tmdb_id    INTEGER NOT NULL,
	rating_key TEXT NOT NULL,
	section    TEXT NOT NULL,
	title      TEXT NOT NULL DEFAULT '',
	added_at   INTEGER NOT NULL DEFAULT 0, -- unix seconds, from Plex
	seen_at    TEXT NOT NULL,              -- last scan that saw it
	PRIMARY KEY (type, tmdb_id)
);
