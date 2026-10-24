-- Added for a Not out yet status that was taken back out; 0030 drops the column again. Kept so every
-- install goes through the same steps.
ALTER TABLE requests ADD COLUMN release TEXT NOT NULL DEFAULT '';
