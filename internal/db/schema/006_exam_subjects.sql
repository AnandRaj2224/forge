-- Forge migration 006: explicit exam-subject membership and ordering.
-- Never edit old migrations; this is additive.
--
-- The global catalogue (subjects -> topics -> lectures) remains the source
-- of truth for learning objects. gov_exam_subjects records, per exam, which
-- global subjects belong to that exam's preparation track and in what order
-- the exam works through them. GOV progression derives from exam order
-- (gov_exams.sort_order) plus this membership; the global subject order no
-- longer determines exam progression.
--
-- Backfill: materialize membership for pre-existing rows from the syllabus
-- (distinct syllabus subjects per exam, ordered by global subject order) so
-- migrated databases keep a deterministic exam subject order.

CREATE TABLE IF NOT EXISTS gov_exam_subjects (
    exam_id TEXT NOT NULL REFERENCES gov_exams(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    subject_id TEXT NOT NULL REFERENCES gov_subjects(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
    sort_order INTEGER NOT NULL,
    PRIMARY KEY (exam_id, subject_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gov_exam_subjects_exam_sort ON gov_exam_subjects(exam_id, sort_order);
CREATE INDEX IF NOT EXISTS idx_gov_exam_subjects_subject ON gov_exam_subjects(subject_id);

INSERT OR IGNORE INTO gov_exam_subjects (exam_id, subject_id, sort_order)
SELECT ranked.exam_id, ranked.subject_id, ranked.sort_order FROM (
    SELECT e.exam_id AS exam_id, e.subject_id AS subject_id,
        ROW_NUMBER() OVER (PARTITION BY e.exam_id ORDER BY s.sort_order) AS sort_order
    FROM (
        SELECT DISTINCT x.exam_id AS exam_id, t.subject_id AS subject_id
        FROM gov_exam_syllabus x JOIN gov_topics t ON t.id = x.topic_id
    ) e JOIN gov_subjects s ON s.id = e.subject_id
) ranked;
