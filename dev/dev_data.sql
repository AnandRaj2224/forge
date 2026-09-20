-- Forge development fixture.
-- One-time fixture for a fresh database AFTER migrations.
-- Rich mixed state for TUI/engine development; NOT the final static seed/import architecture.
-- No destructive statements. Do not edit schema_migrations.
-- Dates/timestamps are centered around September 2026 (Asia/Kolkata).

PRAGMA foreign_keys = ON;
BEGIN;

-- Settings
INSERT INTO forge_settings (id, timezone, day_start_minute, created_at) VALUES (1, 'Asia/Kolkata', 600, 1788237000);

-- GOV subjects
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-quant', 'Quant', 1);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-reasoning', 'Reasoning', 2);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-english', 'English', 3);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-history', 'History', 4);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-geography', 'Geography', 5);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-ga', 'General Awareness', 6);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-computer', 'Computer Awareness', 7);
INSERT INTO gov_subjects (id, name, sort_order) VALUES ('subj-science', 'Science', 8);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-percentage', 'subj-quant', 'Percentage', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-percentage-01', 'topic-percentage', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-percentage-02', 'topic-percentage', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-ratio', 'subj-quant', 'Ratio & Proportion', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-ratio-01', 'topic-ratio', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-ratio-02', 'topic-ratio', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-number-system', 'subj-quant', 'Number System', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-number-system-01', 'topic-number-system', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-number-system-02', 'topic-number-system', 'Worked Examples', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-number-system-03', 'topic-number-system', 'Advanced Applications', 3);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-analogy', 'subj-reasoning', 'Analogy', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-analogy-01', 'topic-analogy', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-analogy-02', 'topic-analogy', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-series', 'subj-reasoning', 'Number/Alphabet Series', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-series-01', 'topic-series', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-series-02', 'topic-series', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-coding-decoding', 'subj-reasoning', 'Coding-Decoding', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-coding-decoding-01', 'topic-coding-decoding', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-coding-decoding-02', 'topic-coding-decoding', 'Worked Examples', 2);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-grammar', 'subj-english', 'Grammar', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-grammar-01', 'topic-grammar', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-grammar-02', 'topic-grammar', 'Worked Examples', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-grammar-03', 'topic-grammar', 'Advanced Applications', 3);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-vocab', 'subj-english', 'Vocabulary', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-vocab-01', 'topic-vocab', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-vocab-02', 'topic-vocab', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-reading', 'subj-english', 'Reading Comprehension', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-reading-01', 'topic-reading', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-reading-02', 'topic-reading', 'Worked Examples', 2);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-ancient', 'subj-history', 'Ancient India', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-ancient-01', 'topic-ancient', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-ancient-02', 'topic-ancient', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-medieval', 'subj-history', 'Medieval India', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-medieval-01', 'topic-medieval', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-medieval-02', 'topic-medieval', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-modern', 'subj-history', 'Modern India', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-modern-01', 'topic-modern', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-modern-02', 'topic-modern', 'Worked Examples', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-modern-03', 'topic-modern', 'Advanced Applications', 3);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-physical-geo', 'subj-geography', 'Physical Geography', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-physical-geo-01', 'topic-physical-geo', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-physical-geo-02', 'topic-physical-geo', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-indian-geo', 'subj-geography', 'Indian Geography', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-indian-geo-01', 'topic-indian-geo', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-indian-geo-02', 'topic-indian-geo', 'Worked Examples', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-indian-geo-03', 'topic-indian-geo', 'Advanced Applications', 3);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-climate', 'subj-geography', 'Climate & Monsoon', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-climate-01', 'topic-climate', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-climate-02', 'topic-climate', 'Worked Examples', 2);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-polity', 'subj-ga', 'Indian Polity', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-polity-01', 'topic-polity', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-polity-02', 'topic-polity', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-economy', 'subj-ga', 'Indian Economy', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-economy-01', 'topic-economy', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-economy-02', 'topic-economy', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-awards', 'subj-ga', 'Awards & Current Facts', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-awards-01', 'topic-awards', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-awards-02', 'topic-awards', 'Worked Examples', 2);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-hardware', 'subj-computer', 'Computer Hardware', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-hardware-01', 'topic-hardware', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-hardware-02', 'topic-hardware', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-os-basics', 'subj-computer', 'Operating Systems', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-os-basics-01', 'topic-os-basics', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-os-basics-02', 'topic-os-basics', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-networking', 'subj-computer', 'Computer Networks', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-networking-01', 'topic-networking', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-networking-02', 'topic-networking', 'Worked Examples', 2);

INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-physics', 'subj-science', 'Physics Basics', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-physics-01', 'topic-physics', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-physics-02', 'topic-physics', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-chemistry', 'subj-science', 'Chemistry Basics', 2);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-chemistry-01', 'topic-chemistry', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-chemistry-02', 'topic-chemistry', 'Worked Examples', 2);
INSERT INTO gov_topics (id, subject_id, name, sort_order) VALUES ('topic-biology', 'subj-science', 'Biology Basics', 3);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-biology-01', 'topic-biology', 'Core Concepts', 1);
INSERT INTO gov_lectures (id, topic_id, title, sort_order) VALUES ('lec-biology-02', 'topic-biology', 'Worked Examples', 2);

-- GOV exams
INSERT INTO gov_exams (id, name, sort_order) VALUES ('exam-afcat', 'AFCAT', 1);
INSERT INTO gov_exams (id, name, sort_order) VALUES ('exam-ssc-cgl', 'SSC CGL', 2);
INSERT INTO gov_exams (id, name, sort_order) VALUES ('exam-sbi-clerk', 'SBI Clerk', 3);
INSERT INTO gov_exams (id, name, sort_order) VALUES ('exam-ibps-po', 'IBPS PO', 4);

INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-percentage', 1);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-ratio', 2);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-number-system', 3);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-analogy', 4);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-series', 5);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-grammar', 6);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-ancient', 7);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-modern', 8);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-physical-geo', 9);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-indian-geo', 10);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-polity', 11);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-afcat', 'topic-physics', 12);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-percentage', 1);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-ratio', 2);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-number-system', 3);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-analogy', 4);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-series', 5);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-coding-decoding', 6);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-grammar', 7);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-vocab', 8);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-ancient', 9);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-medieval', 10);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-modern', 11);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-indian-geo', 12);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-climate', 13);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-polity', 14);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-economy', 15);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-hardware', 16);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-physics', 17);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ssc-cgl', 'topic-chemistry', 18);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-percentage', 1);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-ratio', 2);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-analogy', 3);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-coding-decoding', 4);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-grammar', 5);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-vocab', 6);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-reading', 7);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-modern', 8);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-indian-geo', 9);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-polity', 10);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-economy', 11);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-awards', 12);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-hardware', 13);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-sbi-clerk', 'topic-networking', 14);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-percentage', 1);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-number-system', 2);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-series', 3);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-coding-decoding', 4);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-grammar', 5);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-reading', 6);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-modern', 7);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-physical-geo', 8);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-climate', 9);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-polity', 10);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-economy', 11);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-awards', 12);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-os-basics', 13);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-networking', 14);
INSERT INTO gov_exam_syllabus (exam_id, topic_id, sort_order) VALUES ('exam-ibps-po', 'topic-biology', 15);

-- GOV routines
INSERT INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (1, 3600);
INSERT INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (2, 3600);
INSERT INTO gov_lecture_routine (slot_order, duration_seconds) VALUES (3, 3600);
INSERT INTO gov_practice_routine (slot_order, duration_seconds, selection_mode, fixed_subject_id) VALUES (1, 3600, 'FIXED_SUBJECT', 'subj-quant');
INSERT INTO gov_practice_routine (slot_order, duration_seconds, selection_mode, fixed_subject_id) VALUES (2, 3600, 'FIXED_SUBJECT', 'subj-reasoning');
INSERT INTO gov_practice_routine (slot_order, duration_seconds, selection_mode, fixed_subject_id) VALUES (3, 3600, 'ROUND_ROBIN', NULL);

-- DSA catalogue: 40 problems
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-001', 'Two Sum', 'Hash Map', 'EASY', 1);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-002', 'Valid Anagram', 'Hash Map', 'EASY', 2);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-003', 'Contains Duplicate', 'Hash Set', 'EASY', 3);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-004', 'Group Anagrams', 'Hash Map', 'MEDIUM', 4);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-005', 'Top K Frequent Elements', 'Heap', 'MEDIUM', 5);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-006', 'Product of Array Except Self', 'Prefix/Suffix', 'MEDIUM', 6);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-007', 'Valid Sudoku', 'Matrix', 'MEDIUM', 7);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-008', 'Encode and Decode Strings', 'Design', 'MEDIUM', 8);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-009', 'Longest Consecutive Sequence', 'Hash Set', 'MEDIUM', 9);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-010', 'Valid Palindrome', 'Two Pointers', 'EASY', 10);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-011', 'Two Sum II', 'Two Pointers', 'MEDIUM', 11);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-012', '3Sum', 'Two Pointers', 'MEDIUM', 12);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-013', 'Container With Most Water', 'Two Pointers', 'MEDIUM', 13);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-014', 'Trapping Rain Water', 'Two Pointers', 'HARD', 14);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-015', 'Best Time to Buy and Sell Stock', 'Sliding Window', 'EASY', 15);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-016', 'Longest Substring Without Repeating Characters', 'Sliding Window', 'MEDIUM', 16);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-017', 'Longest Repeating Character Replacement', 'Sliding Window', 'MEDIUM', 17);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-018', 'Permutation in String', 'Sliding Window', 'MEDIUM', 18);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-019', 'Minimum Window Substring', 'Sliding Window', 'HARD', 19);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-020', 'Valid Parentheses', 'Stack', 'EASY', 20);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-021', 'Min Stack', 'Stack', 'MEDIUM', 21);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-022', 'Evaluate Reverse Polish Notation', 'Stack', 'MEDIUM', 22);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-023', 'Daily Temperatures', 'Monotonic Stack', 'MEDIUM', 23);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-024', 'Car Fleet', 'Monotonic Stack', 'MEDIUM', 24);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-025', 'Binary Search', 'Binary Search', 'EASY', 25);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-026', 'Search a 2D Matrix', 'Binary Search', 'MEDIUM', 26);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-027', 'Koko Eating Bananas', 'Binary Search', 'MEDIUM', 27);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-028', 'Find Minimum in Rotated Sorted Array', 'Binary Search', 'MEDIUM', 28);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-029', 'Search in Rotated Sorted Array', 'Binary Search', 'MEDIUM', 29);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-030', 'Invert Binary Tree', 'Trees', 'EASY', 30);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-031', 'Maximum Depth of Binary Tree', 'Trees', 'EASY', 31);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-032', 'Diameter of Binary Tree', 'Trees', 'EASY', 32);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-033', 'Balanced Binary Tree', 'Trees', 'EASY', 33);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-034', 'Binary Tree Level Order Traversal', 'Trees', 'MEDIUM', 34);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-035', 'Lowest Common Ancestor of a BST', 'Trees', 'MEDIUM', 35);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-036', 'Kth Smallest Element in a BST', 'Trees', 'MEDIUM', 36);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-037', 'Serialize and Deserialize Binary Tree', 'Trees', 'HARD', 37);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-038', 'Number of Islands', 'Graphs', 'MEDIUM', 38);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-039', 'Clone Graph', 'Graphs', 'MEDIUM', 39);
INSERT INTO dsa_problems (id, title, pattern, difficulty, sort_order) VALUES ('dsa-040', 'Course Schedule', 'Graphs', 'MEDIUM', 40);
INSERT INTO dsa_routine (id, planned_seconds) VALUES (1, 9000);

-- SWE catalogue: 8 areas x 5 tasks
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-cn', 'Computer Networks', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-cn-t1', 'swe-cn', 'OSI vs TCP/IP layers', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-cn-t2', 'swe-cn', 'HTTP/1.1 request lifecycle', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-cn-t3', 'swe-cn', 'HTTP/2 multiplexing', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-cn-t4', 'swe-cn', 'TCP connection management', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-cn-t5', 'swe-cn', 'DNS resolution and caching', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-sql', 'SQL & Postgres', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-sql-t1', 'swe-sql', 'SELECT/JOIN fundamentals', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-sql-t2', 'swe-sql', 'Indexes and query plans', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-sql-t3', 'swe-sql', 'Transactions and isolation', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-sql-t4', 'swe-sql', 'Postgres constraints', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-sql-t5', 'swe-sql', 'Window functions', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-docker', 'Docker', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-docker-t1', 'swe-docker', 'Container vs image', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-docker-t2', 'swe-docker', 'Dockerfile layering', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-docker-t3', 'swe-docker', 'Volumes and bind mounts', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-docker-t4', 'swe-docker', 'Compose networking', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-docker-t5', 'swe-docker', 'Multi-stage builds', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-os', 'Operating Systems', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-os-t1', 'swe-os', 'Processes vs threads', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-os-t2', 'swe-os', 'Virtual memory', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-os-t3', 'swe-os', 'Scheduling basics', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-os-t4', 'swe-os', 'File descriptors', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-os-t5', 'swe-os', 'Synchronization primitives', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-git', 'Git', 5);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-git-t1', 'swe-git', 'Commit graph and refs', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-git-t2', 'swe-git', 'Interactive rebase', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-git-t3', 'swe-git', 'Cherry-pick and revert', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-git-t4', 'swe-git', 'Bisect workflow', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-git-t5', 'swe-git', 'Conflict resolution', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-linux', 'Linux', 6);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-linux-t1', 'swe-linux', 'Filesystem navigation', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-linux-t2', 'swe-linux', 'Permissions and ownership', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-linux-t3', 'swe-linux', 'systemd services', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-linux-t4', 'swe-linux', 'Processes and signals', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-linux-t5', 'swe-linux', 'Networking tools', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-go', 'Go', 7);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-go-t1', 'swe-go', 'Interfaces and method sets', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-go-t2', 'swe-go', 'Goroutines and channels', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-go-t3', 'swe-go', 'Context cancellation', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-go-t4', 'swe-go', 'Error handling patterns', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-go-t5', 'swe-go', 'HTTP server structure', 5);
INSERT INTO swe_areas (id, name, sort_order) VALUES ('swe-devops', 'CI/CD & Cloud', 8);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-devops-t1', 'swe-devops', 'GitHub Actions basics', 1);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-devops-t2', 'swe-devops', 'Container registry flow', 2);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-devops-t3', 'swe-devops', 'Azure VM deployment', 3);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-devops-t4', 'swe-devops', 'Secrets and environment config', 4);
INSERT INTO swe_tasks (id, area_id, title, sort_order) VALUES ('swe-devops-t5', 'swe-devops', 'Health checks and rollback', 5);

-- SWE weekly routine: 2 x 1h per day
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-1-1', 1, 0, 'swe-cn', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-1-2', 1, 1, 'swe-sql', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-2-1', 2, 0, 'swe-docker', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-2-2', 2, 1, 'swe-os', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-3-1', 3, 0, 'swe-git', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-3-2', 3, 1, 'swe-linux', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-4-1', 4, 0, 'swe-go', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-4-2', 4, 1, 'swe-sql', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-5-1', 5, 0, 'swe-cn', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-5-2', 5, 1, 'swe-docker', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-6-1', 6, 0, 'swe-linux', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-6-2', 6, 1, 'swe-go', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-7-1', 7, 0, 'swe-git', 3600);
INSERT INTO swe_routine (id, weekday, position, area_id, duration_seconds) VALUES ('swr-7-2', 7, 1, 'swe-devops', 3600);

-- GOV runtime state
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-quant', 1787629200);
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-reasoning', 1787633100);
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-english', 1787715300);
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-history', 1788415200);
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-geography', 1788839400);
INSERT INTO gov_subject_activation (subject_id, activated_at) VALUES ('subj-ga', 1789449000);
INSERT INTO gov_practice_rotation_state (id, last_subject_id, updated_at) VALUES (1, 'subj-history', 1789820400);

-- Today snapshots
INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES ('today-2026-09-18-r1', '2026-09-18', 1, 1789704000);
INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES ('today-2026-09-19-r1', '2026-09-19', 1, 1789790100);
INSERT INTO today_snapshots (id, local_date, revision, generated_at) VALUES ('today-2026-09-20-r1', '2026-09-20', 1, 1789876200);

INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-01', 'today-2026-09-18-r1', 1, NULL, 'GOV', 'LECTURE', 'Quant • Percentage • Core Concepts', 3600, '{"subject_id":"subj-quant","topic_id":"topic-percentage","lecture_id":"lec-percentage-01"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-02', 'today-2026-09-18-r1', 2, NULL, 'GOV', 'LECTURE', 'Reasoning • Analogy • Core Concepts', 3600, '{"subject_id":"subj-reasoning","topic_id":"topic-analogy","lecture_id":"lec-analogy-01"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-03', 'today-2026-09-18-r1', 3, NULL, 'GOV', 'PRACTICE', 'Quant practice • Percentages', 3600, '{"subject_id":"subj-quant","topic_ids":["topic-percentage","topic-ratio","topic-number-system"]}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-04', 'today-2026-09-18-r1', 4, NULL, 'DSA', 'DSA', 'DSA • Two Sum', 7200, '{"problem_id":"dsa-001","mode":"NEW"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-05', 'today-2026-09-18-r1', 5, NULL, 'SWE', 'SWE', 'Computer Networks • HTTP/1.1 request lifecycle', 2400, '{"area_id":"swe-cn","task_id":"swe-cn-t2"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-18-r1-task-06', 'today-2026-09-18-r1', 6, NULL, 'SWE', 'SWE', 'SQL & Postgres • SELECT/JOIN fundamentals', 2400, '{"area_id":"swe-sql","task_id":"swe-sql-t1"}');

INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-01', 'today-2026-09-19-r1', 1, NULL, 'GOV', 'LECTURE', 'English • Grammar • Advanced Applications', 3600, '{"subject_id":"subj-english","topic_id":"topic-grammar","lecture_id":"lec-grammar-03"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-02', 'today-2026-09-19-r1', 2, NULL, 'GOV', 'LECTURE', 'History • Ancient India • Core Concepts', 3600, '{"subject_id":"subj-history","topic_id":"topic-ancient","lecture_id":"lec-ancient-01"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-03', 'today-2026-09-19-r1', 3, NULL, 'GOV', 'PRACTICE', 'Reasoning practice • Series', 3600, '{"subject_id":"subj-reasoning","topic_ids":["topic-series","topic-coding-decoding","topic-analogy"]}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-04', 'today-2026-09-19-r1', 4, NULL, 'DSA', 'DSA', 'DSA • Product of Array Except Self', 7200, '{"problem_id":"dsa-006","mode":"NEW"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-05', 'today-2026-09-19-r1', 5, NULL, 'SWE', 'SWE', 'Docker • Compose networking', 2400, '{"area_id":"swe-docker","task_id":"swe-docker-t4"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-19-r1-task-06', 'today-2026-09-19-r1', 6, NULL, 'SWE', 'SWE', 'Go • Context cancellation', 2400, '{"area_id":"swe-go","task_id":"swe-go-t3"}');

INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-01', 'today-2026-09-20-r1', 1, 600, 'GOV', 'LECTURE', 'Quant • Number System • Worked Examples', 3600, '{"subject_id":"subj-quant","topic_id":"topic-number-system","lecture_id":"lec-number-system-02"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-02', 'today-2026-09-20-r1', 2, 660, 'GOV', 'LECTURE', 'Reasoning • Coding-Decoding • Core Concepts', 3600, '{"subject_id":"subj-reasoning","topic_id":"topic-coding-decoding","lecture_id":"lec-coding-decoding-01"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-03', 'today-2026-09-20-r1', 3, 720, 'GOV', 'LECTURE', 'History • Modern India • Worked Examples', 3600, '{"subject_id":"subj-history","topic_id":"topic-modern","lecture_id":"lec-modern-02"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-04', 'today-2026-09-20-r1', 4, 780, 'GOV', 'PRACTICE', 'Quant practice • Number System', 3600, '{"subject_id":"subj-quant","topic_ids":["topic-number-system","topic-percentage"]}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-05', 'today-2026-09-20-r1', 5, 840, 'GOV', 'PRACTICE', 'Reasoning practice • Coding-Decoding', 3600, '{"subject_id":"subj-reasoning","topic_ids":["topic-coding-decoding","topic-series"]}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-06', 'today-2026-09-20-r1', 6, 900, 'GOV', 'PRACTICE', 'History practice • Ancient + Modern', 3600, '{"subject_id":"subj-history","topic_ids":["topic-ancient","topic-modern"]}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-07', 'today-2026-09-20-r1', 7, 960, 'DSA', 'DSA', 'DSA • Longest Substring Without Repeating Characters', 7200, '{"problem_id":"dsa-016","mode":"REPEAT"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-08', 'today-2026-09-20-r1', 8, 1080, 'SWE', 'SWE', 'Docker • Multi-stage builds', 2400, '{"area_id":"swe-docker","task_id":"swe-docker-t5"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-09', 'today-2026-09-20-r1', 9, 1140, 'SWE', 'SWE', 'Linux • Processes and signals', 2400, '{"area_id":"swe-linux","task_id":"swe-linux-t4"}');
INSERT INTO today_tasks (id, snapshot_id, position, scheduled_start_minute, domain, kind, title, planned_seconds, target_json) VALUES ('today-2026-09-20-r1-task-10', 'today-2026-09-20-r1', 10, 1200, 'SWE', 'SWE', 'CI/CD & Cloud • Azure VM deployment', 2400, '{"area_id":"swe-devops","task_id":"swe-devops-t3"}');

-- Sessions: historical variety + exactly one active session
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-001', 'today-2026-09-18-r1-task-01', 'GOV', 'LECTURE', 'Quant • Percentage • Core Concepts', '{"subject_id":"subj-quant","topic_id":"topic-percentage","lecture_id":"lec-percentage-01"}', 3600, 1789705800, 1789708620, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-002', 'today-2026-09-18-r1-task-02', 'GOV', 'LECTURE', 'Reasoning • Analogy • Core Concepts', '{"subject_id":"subj-reasoning","topic_id":"topic-analogy","lecture_id":"lec-analogy-01"}', 3600, 1789709400, 1789713300, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-003', 'today-2026-09-18-r1-task-03', 'GOV', 'PRACTICE', 'Quant practice • Percentages', '{"subject_id":"subj-quant","topic_ids":["topic-percentage","topic-ratio","topic-number-system"]}', 3600, 1789714200, 1789717080, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-004', 'today-2026-09-18-r1-task-04', 'DSA', 'DSA', 'DSA • Two Sum', '{"problem_id":"dsa-001","mode":"NEW"}', 7200, 1789723800, 1789727700, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-005', 'today-2026-09-18-r1-task-05', 'SWE', 'SWE', 'Computer Networks • HTTP/1.1 request lifecycle', '{"area_id":"swe-cn","task_id":"swe-cn-t2"}', 2400, 1789731000, 1789732680, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-006', 'today-2026-09-18-r1-task-06', 'SWE', 'SWE', 'SQL & Postgres • SELECT/JOIN fundamentals', '{"area_id":"swe-sql","task_id":"swe-sql-t1"}', 2400, 1789734600, 1789737240, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-007', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Grammar • Advanced Applications', '{"subject_id":"subj-english","topic_id":"topic-grammar","lecture_id":"lec-grammar-03"}', 3600, 1789792200, 1789795920, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-008', 'today-2026-09-19-r1-task-02', 'GOV', 'LECTURE', 'History • Ancient India • Core Concepts', '{"subject_id":"subj-history","topic_id":"topic-ancient","lecture_id":"lec-ancient-01"}', 3600, 1789796400, 1789798860, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-009', 'today-2026-09-19-r1-task-03', 'GOV', 'PRACTICE', 'Reasoning practice • Series', '{"subject_id":"subj-reasoning","topic_ids":["topic-series","topic-coding-decoding","topic-analogy"]}', 3600, 1789799400, 1789802640, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-010', 'today-2026-09-19-r1-task-04', 'DSA', 'DSA', 'DSA • Product of Array Except Self', '{"problem_id":"dsa-006","mode":"NEW"}', 7200, 1789810200, 1789817760, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-011', 'today-2026-09-19-r1-task-05', 'SWE', 'SWE', 'Docker • Compose networking', '{"area_id":"swe-docker","task_id":"swe-docker-t4"}', 2400, 1789819200, 1789821000, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-012', 'today-2026-09-19-r1-task-06', 'SWE', 'SWE', 'Go • Context cancellation', '{"area_id":"swe-go","task_id":"swe-go-t3"}', 2400, 1789821600, 1789823700, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-013', 'today-2026-09-18-r1-task-03', 'GOV', 'PRACTICE', 'Quant practice • Percentages', '{"subject_id":"subj-quant","topic_ids":["topic-percentage"]}', 3600, 1788067800, 1788069120, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-014', 'today-2026-09-18-r1-task-04', 'DSA', 'DSA', 'DSA • Longest Substring Without Repeating Characters', '{"problem_id":"dsa-016","mode":"NEW"}', 7200, 1788345000, 1788349500, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-015', 'today-2026-09-19-r1-task-02', 'GOV', 'LECTURE', 'History • Ancient India • Worked Examples', '{"subject_id":"subj-history","topic_id":"topic-ancient","lecture_id":"lec-ancient-02"}', 3600, 1788582600, 1788586920, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-016', 'today-2026-09-19-r1-task-03', 'GOV', 'PRACTICE', 'Reasoning practice • Series', '{"subject_id":"subj-reasoning","topic_ids":["topic-series"]}', 3600, 1789021800, 1789023660, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-017', 'today-2026-09-19-r1-task-06', 'SWE', 'SWE', 'Go • Context cancellation', '{"area_id":"swe-go","task_id":"swe-go-t3"}', 2400, 1789129800, 1789133220, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-018', 'today-2026-09-18-r1-task-05', 'SWE', 'SWE', 'Computer Networks • HTTP/1.1 request lifecycle', '{"area_id":"swe-cn","task_id":"swe-cn-t2"}', 2400, 1789209000, 1789211460, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-019', 'today-2026-09-18-r1-task-06', 'SWE', 'SWE', 'SQL & Postgres • SELECT/JOIN fundamentals', '{"area_id":"swe-sql","task_id":"swe-sql-t1"}', 2400, 1789471800, 1789472940, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-020', 'today-2026-09-20-r1-task-07', 'DSA', 'DSA', 'DSA • Longest Substring Without Repeating Characters', '{"problem_id":"dsa-016","mode":"REPEAT"}', 7200, 1789551000, 1789556040, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-021', 'today-2026-09-18-r1-task-01', 'GOV', 'LECTURE', 'Quant • Ratio • Core Concepts', '{"subject_id":"subj-quant","topic_id":"topic-ratio","lecture_id":"lec-ratio-01"}', 3600, 1788406200, 1788409500, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-022', 'today-2026-09-18-r1-task-02', 'GOV', 'LECTURE', 'English • Grammar • Core Concepts', '{"subject_id":"subj-english","topic_id":"topic-grammar","lecture_id":"lec-grammar-01"}', 3600, 1788496200, 1788500400, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-023', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Grammar • Worked Examples', '{"subject_id":"subj-english","topic_id":"topic-grammar","lecture_id":"lec-grammar-02"}', 3600, 1788588000, 1788591120, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-024', 'today-2026-09-19-r1-task-02', 'GOV', 'LECTURE', 'History • Modern India • Core Concepts', '{"subject_id":"subj-history","topic_id":"topic-modern","lecture_id":"lec-modern-01"}', 3600, 1788669000, 1788672840, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-025', 'today-2026-09-20-r1-task-01', 'GOV', 'LECTURE', 'Quant • Number System • Worked Examples', '{"subject_id":"subj-quant","topic_id":"topic-number-system","lecture_id":"lec-number-system-02"}', 3600, 1789893600, NULL, 'ACTIVE');

INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-026', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Vocabulary • Core Concepts', '{"subject_id":"subj-english","topic_id":"topic-vocab","lecture_id":"lec-vocab-01"}', 3600, 1788755400, 1788758520, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-027', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Vocabulary • Worked Examples', '{"subject_id":"subj-english","topic_id":"topic-vocab","lecture_id":"lec-vocab-02"}', 3600, 1788841800, 1788844680, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-028', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Reading Comprehension • Core Concepts', '{"subject_id":"subj-english","topic_id":"topic-reading","lecture_id":"lec-reading-01"}', 3600, 1788928200, 1788931560, 'STOPPED');
INSERT INTO sessions (id, today_task_id, domain, kind, title_snapshot, target_snapshot_json, planned_seconds, started_at, stopped_at, status) VALUES ('sess-029', 'today-2026-09-19-r1-task-01', 'GOV', 'LECTURE', 'English • Reading Comprehension • Worked Examples', '{"subject_id":"subj-english","topic_id":"topic-reading","lecture_id":"lec-reading-02"}', 3600, 1789014600, 1789018080, 'STOPPED');

-- GOV lecture completions: English is fully lecture-complete; Quant/Reasoning/History remain partially complete.
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-percentage-01', 'sess-001', 1789708620);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-ratio-01', 'sess-021', 1788409500);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-analogy-01', 'sess-002', 1789713300);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-grammar-01', 'sess-022', 1788500400);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-grammar-02', 'sess-023', 1788591120);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-grammar-03', 'sess-007', 1789795920);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-vocab-01', 'sess-026', 1788758520);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-vocab-02', 'sess-027', 1788844680);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-reading-01', 'sess-028', 1788931560);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-reading-02', 'sess-029', 1789018080);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-ancient-01', 'sess-008', 1789798860);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-ancient-02', 'sess-015', 1788586920);
INSERT INTO gov_lecture_completions (lecture_id, session_id, completed_at) VALUES ('lec-modern-01', 'sess-024', 1788672840);

-- GOV practice evidence: strong/weak/stale records and missing question speeds.
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-001', 'sess-003', 'topic-percentage', 1380, 20, 16, 17.2, 1789716600);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-002', 'sess-003', 'topic-ratio', 1140, 15, 11, 21.6, 1789716840);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-003', 'sess-003', 'topic-number-system', 480, 8, 3, 34.7, 1789716960);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-004', 'sess-009', 'topic-series', 1860, 26, 21, NULL, 1789802100);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-005', 'sess-009', 'topic-coding-decoding', 1080, 16, 9, 26.4, 1789802400);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-006', 'sess-009', 'topic-analogy', 420, 7, 6, 18.2, 1789802520);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-007', 'sess-013', 'topic-percentage', 1320, 18, 9, 31.8, 1788069000);
INSERT INTO gov_practice_records (id, session_id, topic_id, time_spent_seconds, attempted, correct, avg_question_speed_seconds, recorded_at) VALUES ('gpr-008', 'sess-016', 'topic-series', 1680, 24, 14, 20.5, 1789023480);

-- DSA attempts: NEW passed/failed, REPEAT passed/failed, multiple repeats.
-- Extra NEW introductions keep every populated repeat-state problem historically introduced.
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-001', 'sess-004', 'dsa-001', 'NEW', 1789727100, 1, 1, 0, 75);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-002', 'sess-014', 'dsa-016', 'NEW', 1788348900, 0, 0, 1, 480);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-003', 'sess-020', 'dsa-016', 'REPEAT', 1789555200, 1, 1, 0, 260);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-004', 'sess-020', 'dsa-016', 'REPEAT', 1789555800, 0, 0, 1, 390);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-005', 'sess-010', 'dsa-006', 'NEW', 1789817100, 1, 1, 0, 310);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-006', 'sess-010', 'dsa-011', 'REPEAT', 1789814700, 1, 0, 0, 240);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-007', 'sess-010', 'dsa-011', 'REPEAT', 1789816500, 1, 1, 0, 205);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-008', 'sess-020', 'dsa-021', 'REPEAT', 1789553700, 0, 0, 1, 330);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-009', 'sess-010', 'dsa-011', 'NEW', 1789820100, 1, 1, 0, 260);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-010', 'sess-020', 'dsa-021', 'NEW', 1789550100, 1, 0, 0, 340);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-011', 'sess-018', 'dsa-023', 'NEW', 1789210500, 1, 1, 0, 420);
INSERT INTO dsa_attempts (id, session_id, problem_id, mode, attempted_at, independent, optimal, hint_used, solve_time_seconds) VALUES ('datt-012', 'sess-018', 'dsa-030', 'NEW', 1789209900, 1, 1, 0, 380);

-- DSA repeat state: due today, due soon, and later stages.
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-001', 0, 1, '2026-09-20', 1789729200);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-006', 2, 7, '2026-09-22', 1789816800);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-011', 2, 7, '2026-09-22', 1789816800);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-016', 0, 1, '2026-09-20', 1789556100);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-021', 1, 3, '2026-09-21', 1789650000);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-023', 3, 14, '2026-09-27', 1789212600);
INSERT INTO dsa_repeat_state (problem_id, repeat_stage, current_interval_days, next_review_date, updated_at) VALUES ('dsa-030', 5, 30, '2026-10-05', 1788611400);

-- SWE execution evidence: completed, partial, blocked, and repeated tasks.
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-001', 'sess-005', 'swe-cn-t2', 'COMPLETED', 1789732680);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-002', 'sess-006', 'swe-sql-t1', 'PARTIAL', 1789737240);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-003', 'sess-011', 'swe-docker-t4', 'COMPLETED', 1789821000);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-004', 'sess-012', 'swe-go-t3', 'PARTIAL', 1789823700);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-005', 'sess-017', 'swe-go-t3', 'BLOCKED', 1789133220);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-006', 'sess-018', 'swe-cn-t2', 'COMPLETED', 1789211460);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-007', 'sess-019', 'swe-sql-t1', 'COMPLETED', 1789472940);
INSERT INTO swe_executions (id, session_id, task_id, status, recorded_at) VALUES ('swex-008', 'sess-017', 'swe-go-t3', 'PARTIAL', 1789132800);

-- Weekly review snapshots
INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change) VALUES ('2026-W35', '2026-08-24', '2026-08-30', 1788103800, 1788103800, 126000, 111600, -14400, '{"week":"2026-W35","sessions":11,"highlights":["steady lecture completion","good DSA consistency"],"risk":["practice accuracy uneven"]}', 'Lecture blocks were consistent and DSA felt manageable.', 'Protect practice accuracy work; avoid rushing question sets.');
INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change) VALUES ('2026-W36', '2026-08-31', '2026-09-06', 1788709500, 1788709500, 118800, 124200, 5400, '{"week":"2026-W36","sessions":13,"highlights":["more practice volume","SWE Docker work"],"risk":["several overrun sessions"]}', 'The extra practice repetitions exposed weak topics quickly.', 'Cap overruns and keep a clean stopping point for SWE sessions.');
INSERT INTO weekly_reviews (week, week_start, week_end, generated_at, updated_at, planned_seconds, elapsed_seconds, overrun_seconds, report_json, reflection_worked, reflection_change) VALUES ('2026-W37', '2026-09-07', '2026-09-13', 1789312500, 1789312500, 126000, 119400, -6600, '{"week":"2026-W37","sessions":12,"highlights":["history activation","repeat review system used"],"risk":["some blocked SWE execution"]}', 'The routine stayed predictable while new subjects entered rotation.', 'Resolve blockers earlier and preserve review evidence.');

COMMIT;
