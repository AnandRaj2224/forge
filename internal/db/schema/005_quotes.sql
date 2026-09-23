-- Forge migration 005: daily quote infrastructure.
-- Never edit old migrations; this is additive.
-- Real quote data is seeded separately; this only creates infrastructure.

CREATE TABLE IF NOT EXISTS quotes (
    id INTEGER PRIMARY KEY,
    quote TEXT NOT NULL,
    author TEXT NOT NULL
);

-- Single-row daily selection state (id = 1): persists the current quote's
-- local date so restarting Forge on the same day reuses the same quote.
-- This reuses the existing singleton-state pattern (forge_settings id = 1).
CREATE TABLE IF NOT EXISTS daily_quote_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    local_date TEXT NOT NULL,
    quote_id INTEGER REFERENCES quotes(id) ON DELETE SET NULL ON UPDATE RESTRICT,
    quote_text TEXT NOT NULL,
    author TEXT NOT NULL
);
