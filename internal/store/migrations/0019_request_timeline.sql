-- Each status a request moves through, with when, for the details window's timeline. Requests from before
-- this start with when they were made and their status at the time of this update.
CREATE TABLE request_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  request_id INTEGER NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  at TEXT NOT NULL
);
CREATE INDEX request_events_request ON request_events(request_id);
INSERT INTO request_events (request_id, status, at) SELECT id, 'requested', created_at FROM requests;
INSERT INTO request_events (request_id, status, note, at) SELECT id, status, error, updated_at FROM requests WHERE status NOT IN ('pending', 'approved', 'sent');

-- Where a request came from: '' (someone asked), 'redownload' (an admin replaced a bad file of a title nobody
-- had requested, so the new download can be followed like any request).
ALTER TABLE requests ADD COLUMN source TEXT NOT NULL DEFAULT '';
