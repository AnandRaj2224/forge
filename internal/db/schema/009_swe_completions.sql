-- Forge migration 009: explicit SWE task completions.
-- Never edit old migrations; this is additive.
--
-- SWE is no longer time-based: planned time is not completion. A SWE Today
-- task completes only via an explicit YES on STOP (engine persists one row
-- here). NO leaves the task TODO with sessions preserved for restart.
-- One row per Today task (task-level completion, like lecture completions).

CREATE TABLE IF NOT EXISTS swe_task_completions (
    today_task_id TEXT PRIMARY KEY REFERENCES today_tasks(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    completed_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_swe_completions_completed ON swe_task_completions(completed_at);
