-- ============================================================
-- 000002 UP: Add session status + interview_messages table
-- ============================================================

ALTER TABLE interview_sessions
    ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'pending';

CREATE INDEX IF NOT EXISTS idx_interview_sessions_status ON interview_sessions (status);

CREATE TABLE interview_messages (
                                    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
                                    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                    deleted_at   TIMESTAMPTZ,

                                    session_id   UUID        NOT NULL,

                                    role         VARCHAR(20) NOT NULL,
                                    content      TEXT        NOT NULL,

                                    prompt_tokens     INT NOT NULL DEFAULT 0,
                                    completion_tokens INT NOT NULL DEFAULT 0,

                                    sequence_num INT NOT NULL DEFAULT 0,

                                    CONSTRAINT fk_messages_session
                                        FOREIGN KEY (session_id)
                                            REFERENCES interview_sessions (id)
                                            ON DELETE CASCADE
);

CREATE INDEX idx_interview_messages_deleted_at ON interview_messages (deleted_at);
CREATE INDEX idx_interview_messages_session_id ON interview_messages (session_id);