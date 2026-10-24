-- The Not out yet status was taken back out: its requests go back to waiting for a download, and its
-- timeline steps and column go.
UPDATE requests SET status = 'sent' WHERE status = 'notout';
DELETE FROM request_events WHERE status = 'notout';
ALTER TABLE requests DROP COLUMN release;
