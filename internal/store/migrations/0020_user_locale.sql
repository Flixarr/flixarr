-- A person's own region and language (blank = the server's, Settings → General).
ALTER TABLE users ADD COLUMN region TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT '';
