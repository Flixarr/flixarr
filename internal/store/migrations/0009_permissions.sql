-- Per-user permission switches (comma-separated keys, see store/perms.go). Roles are presets that set them.
ALTER TABLE users ADD COLUMN perms TEXT NOT NULL DEFAULT '';
UPDATE users SET perms = 'admin,manageRequests,autoApprove,requestMovies,requestTv,advanced,viewRequests,reportIssues,manageIssues,watchlist' WHERE role = 'Admin';
UPDATE users SET perms = 'requestMovies,requestTv,reportIssues,watchlist', role = 'User' WHERE role != 'Admin';
