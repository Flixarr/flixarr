-- Request limits per user (per 7 days; 0 = unlimited), and how many seasons a series request asked for.
ALTER TABLE users ADD COLUMN movie_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN season_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE requests ADD COLUMN season_count INTEGER NOT NULL DEFAULT 0;
