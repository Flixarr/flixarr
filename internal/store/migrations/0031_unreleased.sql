-- Not released yet (#151): Radarr / Sonarr say what was asked for isn't out, so it can't download yet.
ALTER TABLE requests ADD COLUMN unreleased INTEGER NOT NULL DEFAULT 0;
