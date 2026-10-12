-- Web Push: each browser or phone someone turned notifications on for.
CREATE TABLE push_subs (
  endpoint TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  device TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX push_subs_user ON push_subs(user_id);
