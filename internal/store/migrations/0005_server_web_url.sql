-- The address a browser uses to open Radarr/Sonarr ("Open Radarr"); blank = the connection address.
ALTER TABLE servers ADD COLUMN web_url TEXT NOT NULL DEFAULT '';
