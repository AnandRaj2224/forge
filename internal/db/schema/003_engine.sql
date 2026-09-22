-- Forge migration 003: engine correctness invariants.
-- Never edit old migrations; this is additive.

-- Canonical Forge calendar settings: Asia/Kolkata, midnight day start.
INSERT INTO forge_settings (id, timezone, day_start_minute, created_at)
VALUES (1, 'Asia/Kolkata', 0, 1788237000)
ON CONFLICT(id) DO UPDATE SET timezone = 'Asia/Kolkata', day_start_minute = 0;

-- Explicit GOV subject mapping for mock sections. Nullable to preserve
-- legacy rows (never infer mappings from section names); new/updated
-- sections must provide it via engine validation.
ALTER TABLE mock_sections ADD COLUMN gov_subject_id TEXT REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT;

-- Helpful indexes for engine queries.
CREATE INDEX IF NOT EXISTS idx_sessions_status_started ON sessions(status, started_at);
CREATE INDEX IF NOT EXISTS idx_today_snapshots_date_rev ON today_snapshots(local_date, revision);
CREATE INDEX IF NOT EXISTS idx_mock_sections_subject ON mock_sections(gov_subject_id);
