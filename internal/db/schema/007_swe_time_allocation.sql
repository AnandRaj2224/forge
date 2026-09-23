-- Forge migration 007: SWE time-allocation model.
-- Never edit old migrations; this is additive.
--
-- Removes the SWE task catalogue, task progression, and task outcome
-- machinery. SWE is now a time-allocation system:
--   swe_areas + swe_routine_slots -> Today SWE block (area + duration)
--   -> generic session -> elapsed time.
-- No real SWE data has been seeded yet, so dropping the task-dependent
-- tables is safe. The routine remains authoritative for planned SWE work.

-- Drop execution indexes first (dropped automatically with the table, but
-- explicit for clarity on migrated DBs).
DROP INDEX IF EXISTS idx_swe_executions_task_recorded;
DROP INDEX IF EXISTS idx_swe_executions_session;

DROP TABLE IF EXISTS swe_executions;
DROP TABLE IF EXISTS swe_tasks;

-- Final routine table: swe_routine_slots.
CREATE TABLE IF NOT EXISTS swe_routine_slots (
    id TEXT PRIMARY KEY,
    weekday INTEGER NOT NULL CHECK (weekday >= 1 AND weekday <= 7),
    position INTEGER NOT NULL CHECK (position >= 0),
    area_id TEXT NOT NULL REFERENCES swe_areas(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    UNIQUE (weekday, position)
);

-- Carry any pre-existing routine rows forward (no-op on fresh DBs).
INSERT OR IGNORE INTO swe_routine_slots (id, weekday, position, area_id, duration_seconds)
SELECT id, weekday, position, area_id, duration_seconds FROM swe_routine;

DROP TABLE IF EXISTS swe_routine;
