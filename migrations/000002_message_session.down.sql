-- ============================================================
-- 000002 DOWN: Revert session status + interview_messages table
-- ============================================================

DROP TABLE IF EXISTS interview_messages;

DROP INDEX IF EXISTS idx_interview_sessions_status;

ALTER TABLE interview_sessions
DROP COLUMN IF EXISTS status;