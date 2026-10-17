-- Alerts rework (#90): one alert per thing. News (Ready to watch, Declined…) or a to-do that stays until
-- it's handled (waiting for approval, a new issue…). The old grouped alerts don't fit and are dropped.
DELETE FROM alerts;
ALTER TABLE alerts ADD COLUMN kind TEXT NOT NULL DEFAULT 'news'; -- news | todo
ALTER TABLE alerts ADD COLUMN poster TEXT NOT NULL DEFAULT '';
-- When each person last opened the bell: alerts newer than this are "new" (the red).
ALTER TABLE users ADD COLUMN bell_opened_at TEXT NOT NULL DEFAULT '';
