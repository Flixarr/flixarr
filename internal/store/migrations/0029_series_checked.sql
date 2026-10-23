-- Whole or part of a series (#137, #165): when it was last checked and whether the show has ended, so
-- ended shows are checked weekly instead of daily.
ALTER TABLE plex_items ADD COLUMN eps_checked TEXT NOT NULL DEFAULT '';
ALTER TABLE plex_items ADD COLUMN eps_ended INTEGER NOT NULL DEFAULT 0;
