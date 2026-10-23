-- Not out yet (#151): when an unreleased request comes out, as "kind:date" (digital:2026-11-12,
-- theaters:2026-10-03, disc:…, air:2027-01-08, or "none:" with no date yet).
ALTER TABLE requests ADD COLUMN release TEXT NOT NULL DEFAULT '';
