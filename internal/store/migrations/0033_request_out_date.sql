-- Not out yet (#300): the date what was asked for comes out to download (Radarr's digital, else disc
-- date; a series season's air date), '' when unknown. Stuck (#299): some of a series is already in Plex.
ALTER TABLE requests ADD COLUMN out_date TEXT NOT NULL DEFAULT '';
ALTER TABLE requests ADD COLUMN some_ready INTEGER NOT NULL DEFAULT 0;
