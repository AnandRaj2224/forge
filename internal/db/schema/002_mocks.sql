-- Forge mocks: actual completed mock/exam attempts as runtime evidence.
-- A mock belongs to ONE gov exam. Weak topics reference global gov_topics by ID
-- (no free-text duplicates). Up to 3 weak topics PER SECTION, ordered 1..3.
-- Mock date preserves recency for the future practice engine (no decay formula yet).

CREATE TABLE IF NOT EXISTS mocks (
    id TEXT PRIMARY KEY,
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    name TEXT NOT NULL,
    mock_date TEXT NOT NULL,
    platform TEXT NOT NULL DEFAULT '',
    score INTEGER CHECK (score IS NULL OR score >= 0),
    total_score INTEGER CHECK (total_score IS NULL OR total_score > 0),
    rank_value INTEGER CHECK (rank_value IS NULL OR rank_value > 0),
    percentile REAL CHECK (percentile IS NULL OR (percentile >= 0 AND percentile <= 100)),
    accuracy REAL CHECK (accuracy IS NULL OR (accuracy >= 0 AND accuracy <= 100)),
    avg_speed_seconds REAL CHECK (avg_speed_seconds IS NULL OR avg_speed_seconds > 0),
    attempted INTEGER CHECK (attempted IS NULL OR attempted >= 0),
    total_questions INTEGER CHECK (total_questions IS NULL OR total_questions > 0),
    correct INTEGER CHECK (correct IS NULL OR correct >= 0),
    incorrect INTEGER CHECK (incorrect IS NULL OR incorrect >= 0),
    unattempted INTEGER CHECK (unattempted IS NULL OR unattempted >= 0),
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS mock_sections (
    id TEXT PRIMARY KEY,
    mock_id TEXT NOT NULL REFERENCES mocks(id) ON DELETE CASCADE ON UPDATE RESTRICT,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    UNIQUE (mock_id, name),
    UNIQUE (mock_id, sort_order)
);

CREATE TABLE IF NOT EXISTS mock_weak_topics (
    section_id TEXT NOT NULL REFERENCES mock_sections(id) ON DELETE CASCADE ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    rank_order INTEGER NOT NULL CHECK (rank_order >= 1 AND rank_order <= 3),
    PRIMARY KEY (section_id, topic_id),
    UNIQUE (section_id, rank_order)
);

CREATE INDEX IF NOT EXISTS idx_mocks_exam_date ON mocks(exam_id, mock_date);
CREATE INDEX IF NOT EXISTS idx_mocks_date ON mocks(mock_date);
CREATE INDEX IF NOT EXISTS idx_mock_sections_mock ON mock_sections(mock_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_mock_weak_topics_section ON mock_weak_topics(section_id, rank_order);
