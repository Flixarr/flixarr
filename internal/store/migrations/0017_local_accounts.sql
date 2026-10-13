-- Local accounts (username and password, no Plex): plex_id 0 and a password hash.
ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX users_local_name ON users(lower(username)) WHERE plex_id = 0;
