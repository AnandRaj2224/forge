-- Forge migration 008: daily quote non-repeat cycle.
-- Never edit old migrations; this is additive.
--
-- Adds a per-quote cycle flag so a quote is never selected again until
-- every quote has been used once. 0 = not used in the current cycle,
-- 1 = already shown in the current cycle. New rows default to unused.

ALTER TABLE quotes ADD COLUMN used INTEGER NOT NULL DEFAULT 0 CHECK (used IN (0,1));
