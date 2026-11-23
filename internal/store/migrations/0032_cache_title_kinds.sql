-- Saved titles are split into movies, series, seasons and collections (Settings, Cache).
UPDATE cache SET kind = CASE
  WHEN key LIKE 'collection/%' THEN 'collections'
  WHEN key LIKE 'series/%/season/%' THEN 'seasons'
  WHEN key LIKE 'series/%' THEN 'series'
  WHEN key LIKE 'contributors%' THEN 'lists'
  ELSE 'movies'
END
WHERE kind = 'titles';
