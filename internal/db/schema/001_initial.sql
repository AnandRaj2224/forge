-- Forge initial schema: static catalogue + runtime evidence.
-- Timestamps: INTEGER Unix seconds UTC. Dates: TEXT YYYY-MM-DD. Weeks: TEXT YYYY-Www.

-- Singleton settings.
CREATE TABLE IF NOT EXISTS forge_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    timezone TEXT NOT NULL,
    day_start_minute INTEGER NOT NULL CHECK (day_start_minute >= 0 AND day_start_minute < 1440),
    created_at INTEGER NOT NULL
);

-- Gov static.
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

CREATE TABLE IF NOT EXISTS gov_exam_syllabus (
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (exam_id, topic_id)
);

CREATE TABLE IF NOT EXISTS gov_lecture_routine (
    slot_order INTEGER PRIMARY KEY,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0)
);

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

-- DSA static.
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

-- SWE static.
CREATE TABLE IF NOT EXISTS swe_areas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    sort_order INTEGER NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS swe_tasks (
    id TEXT PRIMARY KEY,
    area_id TEXT NOT NULL REFERENCES swe_areas(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    title TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    UNIQUE (area_id, sort_order)
);

CREATE TABLE IF NOT EXISTS swe_routine (
    id TEXT PRIMARY KEY,
    weekday INTEGER NOT NULL CHECK (weekday >= 1 AND weekday <= 7),
    position INTEGER NOT NULL CHECK (position >= 0),
    area_id TEXT NOT NULL REFERENCES swe_areas(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    duration_seconds INTEGER NOT NULL CHECK (duration_seconds > 0),
    UNIQUE (weekday, position)
);

-- Today snapshots (must exist before sessions).
CREATE TABLE IF NOT EXISTS today_snapshots (
    id TEXT PRIMARY KEY,
    local_date TEXT NOT NULL,
    revision INTEGER NOT NULL,
    generated_at INTEGER NOT NULL,
    UNIQUE (local_date, revision)
);

CREATE TABLE IF NOT EXISTS today_tasks (
    id TEXT PRIMARY KEY,
    snapshot_id TEXT NOT NULL REFERENCES today_snapshots(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    position INTEGER NOT NULL,
    scheduled_start_minute INTEGER,
    domain TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    planned_seconds INTEGER NOT NULL,
    target_json TEXT,
    UNIQUE (snapshot_id, position)
);

-- Sessions: one row per concrete Today task execution.
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

CREATE TABLE IF NOT EXISTS gov_practice_records (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    topic_id TEXT NOT NULL REFERENCES gov_topics(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    time_spent_seconds INTEGER NOT NULL CHECK (time_spent_seconds >= 0),
    attempted INTEGER NOT NULL CHECK (attempted >= 0),
    correct INTEGER NOT NULL CHECK (correct >= 0),
    avg_question_speed_seconds REAL CHECK (avg_question_speed_seconds IS NULL OR avg_question_speed_seconds > 0),
    recorded_at INTEGER NOT NULL,
    CHECK (correct <= attempted),
    UNIQUE (session_id, topic_id)
);

CREATE TABLE IF NOT EXISTS gov_practice_rotation_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_subject_id TEXT REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    updated_at INTEGER NOT NULL
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

CREATE TABLE IF NOT EXISTS dsa_repeat_state (
    problem_id TEXT PRIMARY KEY REFERENCES dsa_problems(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    repeat_stage INTEGER NOT NULL CHECK (repeat_stage >= 0),
    current_interval_days INTEGER NOT NULL CHECK (current_interval_days > 0),
    next_review_date TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

-- SWE runtime.
CREATE TABLE IF NOT EXISTS swe_executions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    task_id TEXT NOT NULL REFERENCES swe_tasks(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('COMPLETED', 'PARTIAL', 'BLOCKED')),
    recorded_at INTEGER NOT NULL
);

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

-- Indexes for normal application queries.
-- NOTE: no explicit indexes are created where SQLite already maintains an
-- identical automatic unique index (e.g. UNIQUE(subject_id, sort_order)).
CREATE INDEX IF NOT EXISTS idx_gov_exam_syllabus_exam_sort ON gov_exam_syllabus(exam_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_gov_exam_syllabus_topic ON gov_exam_syllabus(topic_id);
CREATE INDEX IF NOT EXISTS idx_gov_lecture_completions_completed ON gov_lecture_completions(completed_at);
CREATE INDEX IF NOT EXISTS idx_gov_practice_records_topic_recorded ON gov_practice_records(topic_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_gov_practice_records_session ON gov_practice_records(session_id);
CREATE INDEX IF NOT EXISTS idx_dsa_attempts_problem_attempted ON dsa_attempts(problem_id, attempted_at);
CREATE INDEX IF NOT EXISTS idx_dsa_attempts_session ON dsa_attempts(session_id);
CREATE INDEX IF NOT EXISTS idx_dsa_repeat_state_next_review ON dsa_repeat_state(next_review_date);
CREATE INDEX IF NOT EXISTS idx_swe_executions_task_recorded ON swe_executions(task_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_swe_executions_session ON swe_executions(session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_sessions_today_task ON sessions(today_task_id);
CREATE INDEX IF NOT EXISTS idx_weekly_reviews_week_start ON weekly_reviews(week_start);

-- Partial unique indexes.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dsa_attempts_new_unique ON dsa_attempts(problem_id) WHERE mode = 'NEW';
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_single_active ON sessions(status) WHERE status = 'ACTIVE';
