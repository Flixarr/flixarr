-- Import from Seerr (#95): which rows came from Seerr, so importing again skips them instead of adding twice.
ALTER TABLE users ADD COLUMN seerr_id INTEGER;
ALTER TABLE requests ADD COLUMN seerr_id INTEGER;
ALTER TABLE issues ADD COLUMN seerr_id INTEGER;
CREATE INDEX requests_seerr ON requests (seerr_id) WHERE seerr_id IS NOT NULL;
