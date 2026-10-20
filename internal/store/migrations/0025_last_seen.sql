-- When each person last used Flixarr (#149), shown on Users and their profile. Updated at most every 5 minutes.
ALTER TABLE users ADD COLUMN last_seen TEXT NOT NULL DEFAULT '';
