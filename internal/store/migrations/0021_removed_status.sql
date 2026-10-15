-- Requests whose title was deleted from Plex and Radarr/Sonarr were marked failed with a "Removed from…"
-- reason; they now have their own status, removed.
UPDATE request_events SET status = 'removed' WHERE status = 'failed' AND note LIKE 'Removed from %';
UPDATE requests SET status = 'removed' WHERE status = 'failed' AND error LIKE 'Removed from %';
