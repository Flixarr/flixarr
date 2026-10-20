-- Whole or part of a series in Plex (#137): episodes Plex has of those aired so far (0 total = not checked yet).
ALTER TABLE plex_items ADD COLUMN eps_have INTEGER NOT NULL DEFAULT 0;
ALTER TABLE plex_items ADD COLUMN eps_total INTEGER NOT NULL DEFAULT 0;
