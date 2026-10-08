-- Forge migration 011: add fourth GOV lecture slot.
-- Never edit old migrations; this is additive.
--
-- The GOV lecture routine has 3 lecture slots; add a fourth normal slot.
-- All four slots are identical in semantics: same task type, same 1-hour
-- duration, same activation/completion/UI treatment. No preview slot, no
-- new task type, no special behavior for slot 4.

INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (4, 3600);
