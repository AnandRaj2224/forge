-- Forge V2 consolidated initial schema.
-- Fresh V2 databases initialize from this file only.
-- V1 databases are never migrated by V2; they fail closed via forge_meta.
-- Timestamps: INTEGER Unix seconds UTC. Dates: TEXT YYYY-MM-DD. Weeks: TEXT YYYY-Www.

-- Schema identity: every V2 database carries this marker.
CREATE TABLE IF NOT EXISTS forge_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT OR IGNORE INTO forge_meta (key, value) VALUES ('schema_version', 'v2');
INSERT OR IGNORE INTO forge_meta (key, value) VALUES ('schema_variant', 'forge-v2-consolidated-001');

-- Singleton settings: fixed Forge calendar (Asia/Kolkata, midnight).
CREATE TABLE IF NOT EXISTS forge_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    timezone TEXT NOT NULL,
    day_start_minute INTEGER NOT NULL CHECK (day_start_minute >= 0 AND day_start_minute < 1440),
    created_at INTEGER NOT NULL
);
INSERT OR IGNORE INTO forge_settings (id, timezone, day_start_minute, created_at)
VALUES (1, 'Asia/Kolkata', 0, 1788237000);

-- Gov canonical catalogue.
CREATE TABLE IF NOT EXISTS gov_subjects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS gov_topics (
    id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    UNIQUE (subject_id, sort_order)
);

CREATE TABLE IF NOT EXISTS gov_lectures (
    id TEXT PRIMARY KEY,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    title TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    UNIQUE (topic_id, sort_order)
);

CREATE TABLE IF NOT EXISTS gov_exams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL UNIQUE
);

-- Authoritative exam structure:
-- gov_exam_subjects = which subjects belong to an exam + exam order.
-- gov_exam_syllabus = which canonical topics belong to (exam, subject).
-- The (exam, subject) pair in syllabus must match a gov_exam_subjects row
-- and the topic's canonical subject must equal the syllabus subject.
CREATE TABLE IF NOT EXISTS gov_exam_subjects (
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    subject_id TEXT NOT NULL REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (exam_id, subject_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_gov_exam_subjects_exam_sort ON gov_exam_subjects(exam_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_gov_exam_subjects_subject ON gov_exam_subjects(subject_id);

CREATE TABLE IF NOT EXISTS gov_exam_syllabus (
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    subject_id TEXT REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (exam_id, topic_id),
    UNIQUE (exam_id, subject_id, topic_id)
);
CREATE INDEX IF NOT EXISTS idx_gov_exam_syllabus_exam_sort ON gov_exam_syllabus(exam_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_gov_exam_syllabus_topic ON gov_exam_syllabus(topic_id);
CREATE INDEX IF NOT EXISTS idx_gov_exam_syllabus_exam_subject ON gov_exam_syllabus(exam_id, subject_id);

-- Exam profiles + explicit focus (V2 replaces serial progression).
CREATE TABLE IF NOT EXISTS gov_exam_profiles (
    exam_id TEXT PRIMARY KEY REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'PAUSED', 'ARCHIVED')),
    is_main_goal INTEGER NOT NULL CHECK (is_main_goal IN (0, 1)),
    priority INTEGER NOT NULL,
    target_date TEXT,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gov_exam_profiles_status ON gov_exam_profiles(status);

CREATE TABLE IF NOT EXISTS gov_focus_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    focused_exam_id TEXT REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    updated_at INTEGER NOT NULL
);

-- Routines.
CREATE TABLE IF NOT EXISTS gov_lecture_routine (
    slot_order INTEGER PRIMARY KEY,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0)
);
INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (1, 3600);
INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (2, 3600);
INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (3, 3600);
INSERT OR IGNORE INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (4, 3600);

CREATE TABLE IF NOT EXISTS gov_practice_routine (
    slot_order INTEGER PRIMARY KEY,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    selection_mode TEXT NOT NULL CHECK (selection_mode IN ('FIXED_SUBJECT', 'ROUND_ROBIN')),
    fixed_subject_id TEXT REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    CHECK (
        (selection_mode = 'FIXED_SUBJECT' AND fixed_subject_id IS NOT NULL)
        OR
        (selection_mode = 'ROUND_ROBIN' AND fixed_subject_id IS NULL)
    )
);

-- DSA catalogue.
CREATE TABLE IF NOT EXISTS dsa_problems (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    pattern TEXT NOT NULL,
    difficulty TEXT NOT NULL,
    sort_order INTEGER NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS dsa_routine (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    planned_seconds INTEGER NOT NULL CHECK (planned_seconds > 0)
);

-- SWE time allocation.
CREATE TABLE IF NOT EXISTS swe_areas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS swe_routine_slots (
    id TEXT PRIMARY KEY,
    weekday INTEGER NOT NULL CHECK (weekday >= 1 AND weekday <= 7),
    position INTEGER NOT NULL CHECK (position >= 0),
    area_id TEXT NOT NULL REFERENCES swe_areas(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    UNIQUE (weekday, position)
);

-- TODAY snapshots + immutable tasks + revision membership.
CREATE TABLE IF NOT EXISTS today_snapshots (
    id TEXT PRIMARY KEY,
    local_date TEXT NOT NULL,
    revision INTEGER NOT NULL,
    generated_at INTEGER NOT NULL,
    UNIQUE (local_date, revision)
);
CREATE INDEX IF NOT EXISTS idx_today_snapshots_date_rev ON today_snapshots(local_date, revision);

CREATE TABLE IF NOT EXISTS today_tasks (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES today_snapshots(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    position INTEGER NOT NULL,
    scheduled_start_minute INTEGER,
    domain TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    planned_seconds INTEGER NOT NULL CHECK (planned_seconds > 0),
    target_json TEXT,
    UNIQUE (snapshot_id, position)
);

CREATE TABLE IF NOT EXISTS today_snapshot_tasks (
    snapshot_id TEXT NOT NULL REFERENCES today_snapshots(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    task_id TEXT NOT NULL REFERENCES today_tasks(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    position INTEGER NOT NULL,
    PRIMARY KEY (snapshot_id, task_id),
    UNIQUE (snapshot_id, position)
);
CREATE INDEX IF NOT EXISTS idx_snapshot_tasks_snapshot ON today_snapshot_tasks(snapshot_id, position);
CREATE INDEX IF NOT EXISTS idx_snapshot_tasks_task ON today_snapshot_tasks(task_id);

-- Sessions.
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    today_task_id TEXT NOT NULL REFERENCES today_tasks(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    domain TEXT NOT NULL,
    kind TEXT NOT NULL,
    title_snapshot TEXT NOT NULL,
    target_snapshot_json TEXT,
    planned_seconds INTEGER NOT NULL,
    started_at INTEGER NOT NULL,
    stopped_at INTEGER,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'STOPPED')),
    CHECK (
        (status = 'ACTIVE' AND stopped_at IS NULL)
        OR
        (status = 'STOPPED' AND stopped_at IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_today_task ON sessions(today_task_id);
CREATE INDEX IF NOT EXISTS idx_sessions_task_status ON sessions(today_task_id, status);
CREATE INDEX IF NOT EXISTS idx_sessions_status_started ON sessions(status, started_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_single_active ON sessions(status) WHERE status = 'ACTIVE';

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

CREATE TRIGGER IF NOT EXISTS trg_sessions_stop_order
BEFORE UPDATE OF stopped_at, status ON sessions
FOR EACH ROW WHEN (NEW.stopped_at IS NOT NULL AND NEW.stopped_at < NEW.started_at)
BEGIN
    SELECT RAISE(ABORT, 'sessions.stopped_at must be >= started_at');
END;

-- Gov runtime.
CREATE TABLE IF NOT EXISTS gov_subject_activation (
    subject_id TEXT PRIMARY KEY REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    activated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gov_lecture_completions (
    lecture_id TEXT PRIMARY KEY REFERENCES gov_lectures(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    completed_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gov_lecture_completions_completed ON gov_lecture_completions(completed_at);

CREATE TABLE IF NOT EXISTS gov_practice_records (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    exam_id TEXT REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    time_spent_seconds INTEGER NOT NULL CHECK (time_spent_seconds >= 0),
    attempted INTEGER NOT NULL CHECK (attempted >= 0),
    correct INTEGER NOT NULL CHECK (correct >= 0),
    avg_question_speed_seconds REAL CHECK (avg_question_speed_seconds IS NULL OR avg_question_speed_seconds > 0),
    recorded_at INTEGER NOT NULL,
    CHECK (correct <= attempted),
    UNIQUE (session_id, topic_id)
);
CREATE INDEX IF NOT EXISTS idx_gov_practice_records_topic_recorded ON gov_practice_records(topic_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_gov_practice_records_session ON gov_practice_records(session_id);
CREATE INDEX IF NOT EXISTS idx_gov_practice_records_exam ON gov_practice_records(exam_id, recorded_at);

-- V2 deterministic per-exam+subject topic cursor.
CREATE TABLE IF NOT EXISTS gov_practice_rotation (
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    subject_id TEXT NOT NULL REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    next_index INTEGER NOT NULL CHECK (next_index >= 0),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (exam_id, subject_id)
);

-- DSA runtime.
CREATE TABLE IF NOT EXISTS dsa_attempts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    problem_id TEXT NOT NULL REFERENCES dsa_problems(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    mode TEXT NOT NULL CHECK (mode IN ('NEW', 'REPEAT')),
    attempted_at INTEGER NOT NULL,
    independent INTEGER NOT NULL CHECK (independent IN (0, 1)),
    optimal INTEGER NOT NULL CHECK (optimal IN (0, 1)),
    hint_used INTEGER NOT NULL CHECK (hint_used IN (0, 1)),
    solve_time_seconds INTEGER CHECK (solve_time_seconds IS NULL OR solve_time_seconds >= 0)
);
CREATE INDEX IF NOT EXISTS idx_dsa_attempts_problem_attempted ON dsa_attempts(problem_id, attempted_at);
CREATE INDEX IF NOT EXISTS idx_dsa_attempts_session ON dsa_attempts(session_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dsa_attempts_new_unique ON dsa_attempts(problem_id) WHERE mode = 'NEW';

CREATE TABLE IF NOT EXISTS dsa_repeat_state (
    problem_id TEXT PRIMARY KEY REFERENCES dsa_problems(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    repeat_stage INTEGER NOT NULL CHECK (repeat_stage >= 0),
    current_interval_days INTEGER NOT NULL CHECK (current_interval_days > 0),
    next_review_date TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dsa_repeat_state_next_review ON dsa_repeat_state(next_review_date);

-- SWE completions.
CREATE TABLE IF NOT EXISTS swe_task_completions (
    today_task_id TEXT PRIMARY KEY REFERENCES today_tasks(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    completed_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_swe_completions_completed ON swe_task_completions(completed_at);

-- Mocks.
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
CREATE INDEX IF NOT EXISTS idx_mocks_exam_date ON mocks(exam_id, mock_date);
CREATE INDEX IF NOT EXISTS idx_mocks_date ON mocks(mock_date);

CREATE TABLE IF NOT EXISTS mock_sections (
    id TEXT PRIMARY KEY,
    mock_id TEXT NOT NULL REFERENCES mocks(id) ON DELETE CASCADE ON UPDATE RESTRICT,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    gov_subject_id TEXT REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    UNIQUE (mock_id, name),
    UNIQUE (mock_id, sort_order)
);
CREATE INDEX IF NOT EXISTS idx_mock_sections_mock ON mock_sections(mock_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_mock_sections_subject ON mock_sections(gov_subject_id);

CREATE TABLE IF NOT EXISTS mock_weak_topics (
    section_id TEXT NOT NULL REFERENCES mock_sections(id) ON DELETE CASCADE ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    rank_order INTEGER NOT NULL CHECK (rank_order >= 1 AND rank_order <= 3),
    PRIMARY KEY (section_id, topic_id),
    UNIQUE (section_id, rank_order)
);
CREATE INDEX IF NOT EXISTS idx_mock_weak_topics_section ON mock_weak_topics(section_id, rank_order);

-- Weekly reviews.
CREATE TABLE IF NOT EXISTS weekly_reviews (
    week TEXT PRIMARY KEY,
    week_start TEXT NOT NULL,
    week_end TEXT NOT NULL,
    generated_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    planned_seconds INTEGER NOT NULL,
    elapsed_seconds INTEGER NOT NULL,
    overrun_seconds INTEGER NOT NULL,
    report_json TEXT NOT NULL,
    reflection_worked TEXT NOT NULL DEFAULT '',
    reflection_change TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_weekly_reviews_week_start ON weekly_reviews(week_start);

-- Quotes.
CREATE TABLE IF NOT EXISTS quotes (
    id INTEGER PRIMARY KEY,
    quote TEXT NOT NULL,
    author TEXT NOT NULL,
    used INTEGER NOT NULL DEFAULT 0 CHECK (used IN (0,1))
);

CREATE TABLE IF NOT EXISTS daily_quote_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    local_date TEXT NOT NULL,
    quote_id INTEGER REFERENCES quotes(id) ON DELETE SET NULL ON UPDATE RESTRICT,
    quote_text TEXT NOT NULL,
    author TEXT NOT NULL
);
