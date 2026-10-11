-- Alerts: what the bell shows inside Flixarr (not sent anywhere; that's notifications).
-- One active alert per key (e.g. one per Radarr/Sonarr server); resolved ones are kept for history.
CREATE TABLE alerts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  key TEXT NOT NULL,
  audience TEXT NOT NULL DEFAULT 'admins',
  severity TEXT NOT NULL,
  title TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT '',
  link TEXT NOT NULL DEFAULT '',
  sig TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL,
  changed_at TEXT NOT NULL,
  resolved_at TEXT
);
CREATE INDEX alerts_active ON alerts(key) WHERE resolved_at IS NULL;
CREATE TABLE alert_seen (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  alert_id INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
  seen_at TEXT NOT NULL,
  PRIMARY KEY (user_id, alert_id)
);
