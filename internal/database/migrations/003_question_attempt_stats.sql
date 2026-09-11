CREATE TABLE IF NOT EXISTS question_attempt_stats (
    question_uid TEXT PRIMARY KEY REFERENCES questions(uid) ON DELETE CASCADE,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    correct_count INTEGER NOT NULL DEFAULT 0
);

INSERT INTO question_attempt_stats(question_uid, attempt_count, correct_count)
SELECT question_uid, COUNT(*), COALESCE(SUM(correct), 0)
FROM attempts
GROUP BY question_uid;
