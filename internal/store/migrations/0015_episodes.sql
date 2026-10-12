-- Per-episode series requests: the episodes asked for, as JSON [[season, episode], …].
ALTER TABLE requests ADD COLUMN episodes TEXT NOT NULL DEFAULT '[]';
-- Issues can name several seasons/episodes: JSON [[season, episode], …] (episode 0 = the whole season).
ALTER TABLE issues ADD COLUMN parts TEXT NOT NULL DEFAULT '[]';
