-- Forge migration 004: session lifecycle invariants at the DB level.
-- Never edit old migrations; this is additive.
-- Business rules stay in the Engine; these triggers are backstops so no
-- writer (present or future) can persist an impossible session row.

-- Sessions must plan positive work.
CREATE TRIGGER IF NOT EXISTS trg_sessions_planned_positive_ins
BEFORE INSERT ON sessions
FOR EACH ROW WHEN (NEW.planned_seconds <= 0)
BEGIN
    SELECT RAISE(ABORT, 'sessions.planned_seconds must be > 0');
END;

CREATE TRIGGER IF NOT EXISTS trg_sessions_planned_positive_upd
BEFORE UPDATE OF planned_seconds ON sessions
FOR EACH ROW WHEN (NEW.planned_seconds <= 0)
BEGIN
    SELECT RAISE(ABORT, 'sessions.planned_seconds must be > 0');
END;

-- A stopped session cannot end before it started.
CREATE TRIGGER IF NOT EXISTS trg_sessions_stop_order
BEFORE UPDATE OF stopped_at, status ON sessions
FOR EACH ROW WHEN (NEW.stopped_at IS NOT NULL AND NEW.stopped_at < NEW.started_at)
BEGIN
    SELECT RAISE(ABORT, 'sessions.stopped_at must be >= started_at');
END;
