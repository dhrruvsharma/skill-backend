-- Extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- USERS
-- ============================================================
CREATE TABLE users (
                       id                              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
                       created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                       updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                       deleted_at                      TIMESTAMPTZ,

                       first_name                      VARCHAR(100)  NOT NULL,
                       last_name                       VARCHAR(100)  NOT NULL,
                       email                           VARCHAR(255)  NOT NULL,

                       password_hash                   TEXT,
                       provider                        VARCHAR(50)   NOT NULL DEFAULT 'local',
                       provider_id                     VARCHAR(255),
                       role                            VARCHAR(50)   NOT NULL DEFAULT 'user',
                       is_verified                     BOOLEAN       NOT NULL DEFAULT FALSE,
                       is_active                       BOOLEAN       NOT NULL DEFAULT TRUE,

                       reset_token                     VARCHAR(255),
                       reset_token_expires_at          TIMESTAMP,

                       verification_token              VARCHAR(255),
                       verification_token_expires_at   TIMESTAMP,

                       refresh_token_hash              TEXT,
                       refresh_token_exp               TIMESTAMP,

                       CONSTRAINT users_email_unique UNIQUE (email)
);

CREATE INDEX idx_users_deleted_at          ON users (deleted_at);
CREATE INDEX idx_users_reset_token         ON users (reset_token);
CREATE INDEX idx_users_verification_token  ON users (verification_token);


-- ============================================================
-- PERSONAS
-- ============================================================
CREATE TABLE personas (
                          id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
                          created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                          updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                          deleted_at              TIMESTAMPTZ,

                          user_id                 UUID        NOT NULL,

                          name                    VARCHAR(150) NOT NULL,
                          description             TEXT,
                          is_default              BOOLEAN      NOT NULL DEFAULT FALSE,
                          is_active               BOOLEAN      NOT NULL DEFAULT TRUE,

                          target_role             VARCHAR(150),
                          target_company          VARCHAR(150),
                          experience_years        INT          NOT NULL DEFAULT 0,
                          domain                  VARCHAR(100) NOT NULL DEFAULT 'general',
                          difficulty              VARCHAR(50)  NOT NULL DEFAULT 'medium',

                          skills                  JSONB,
                          system_prompt           TEXT,

                          interview_duration_mins INT     NOT NULL DEFAULT 30,
                          enable_video_proctoring BOOLEAN NOT NULL DEFAULT FALSE,
                          enable_audio_proctoring BOOLEAN NOT NULL DEFAULT FALSE,
                          enable_tab_detection    BOOLEAN NOT NULL DEFAULT TRUE,

                          CONSTRAINT fk_personas_user
                              FOREIGN KEY (user_id)
                                  REFERENCES users (id)
                                  ON DELETE CASCADE
);

CREATE INDEX idx_personas_deleted_at  ON personas (deleted_at);
CREATE INDEX idx_personas_user_id     ON personas (user_id);


-- ============================================================
-- INTERVIEW SESSIONS
-- ============================================================
CREATE TABLE interview_sessions (
                                    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
                                    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                                    deleted_at      TIMESTAMPTZ,

                                    user_id         UUID NOT NULL,
                                    persona_id      UUID,                           -- nullable: persona may be deleted

                                    started_at      TIMESTAMP,
                                    ended_at        TIMESTAMP,
                                    duration_secs   INT  NOT NULL DEFAULT 0,

                                    transcript      TEXT,
                                    ai_report       TEXT,

                                    tab_switch_count  INT     NOT NULL DEFAULT 0,
                                    suspicious_audio  BOOLEAN NOT NULL DEFAULT FALSE,
                                    multiple_faces    BOOLEAN NOT NULL DEFAULT FALSE,
                                    cheating_flags    JSONB,

                                    recording_url   TEXT,

                                    CONSTRAINT fk_sessions_user
                                        FOREIGN KEY (user_id)
                                            REFERENCES users (id)
                                            ON DELETE CASCADE,

                                    CONSTRAINT fk_sessions_persona
                                        FOREIGN KEY (persona_id)
                                            REFERENCES personas (id)
                                            ON DELETE SET NULL
);

CREATE INDEX idx_interview_sessions_deleted_at  ON interview_sessions (deleted_at);
CREATE INDEX idx_interview_sessions_user_id     ON interview_sessions (user_id);
CREATE INDEX idx_interview_sessions_persona_id  ON interview_sessions (persona_id);