-- Forge migration 010: SWE weekly routine refresh + DSA 2+2 is code-only.
-- Never edit old migrations; this is additive.
--
-- DSA 2 past + 2 new is enforced in engine code (planDSABlock +
-- ValidateDSACurrentBundle); no schema change needed.
--
-- SWE changes (routine slots are authoritative for future Today blocks;
-- historical today_tasks/sessions/report_json are untouched):
--   Mon: Go 2h (7200), remove Docker slot
--   Tue: Project-Sentinel unchanged, second slot Docker 30m (1800) instead of Prometheus
--   Wed: no change
--   Thu: AgentBox 1h30 (5400), second slot Prometheus+Grafana 30m (1800)
--   Fri: Computer Networks 2h (7200, full name), remove curl slot
--   Sat: no change
--   Sun: Operating System 1h30 (5400, full name), linux(arch)+setup 30m (1800)
--
-- Areas are kept for history (orphaned prometheus/grafana/curl/docker-Mon
-- rows stay); only slots + the two renamed areas change. All statements
-- are safe on fresh DBs (UPDATE/DELETE affect 0 rows, INSERT uses OR IGNORE).

-- Combined Thursday area (raw lower form; display override renders
-- "Prometheus + Grafana"). Sort 15 is free on prod (max 14) and on fresh.
INSERT OR IGNORE INTO swe_areas (id, name, sort_order) VALUES ('swe-area-prometheus-grafana', 'prometheus + grafana', 15);

-- Full names.
UPDATE swe_areas SET name = 'Computer Networks' WHERE id = 'swe-area-cn';
UPDATE swe_areas SET name = 'Operating System' WHERE id = 'swe-area-os';

-- Monday: Go 2h single slot.
UPDATE swe_routine_slots SET duration_seconds = 7200 WHERE id = 'swe-slot-1-00';
DELETE FROM swe_routine_slots WHERE id = 'swe-slot-1-01';

-- Tuesday: second slot becomes Docker 30m (Sentinel slot untouched).
UPDATE swe_routine_slots SET area_id = 'swe-area-docker', duration_seconds = 1800 WHERE id = 'swe-slot-2-01';

-- Thursday: AgentBox 1h30 + combined 30m.
UPDATE swe_routine_slots SET duration_seconds = 5400 WHERE id = 'swe-slot-4-00';
UPDATE swe_routine_slots SET area_id = 'swe-area-prometheus-grafana', duration_seconds = 1800 WHERE id = 'swe-slot-4-01';

-- Friday: Computer Networks 2h single slot.
UPDATE swe_routine_slots SET duration_seconds = 7200 WHERE id = 'swe-slot-5-00';
DELETE FROM swe_routine_slots WHERE id = 'swe-slot-5-01';

-- Sunday: Operating System 1h30 + linux setup 30m.
UPDATE swe_routine_slots SET duration_seconds = 5400 WHERE id = 'swe-slot-7-00';
UPDATE swe_routine_slots SET duration_seconds = 1800 WHERE id = 'swe-slot-7-01';
