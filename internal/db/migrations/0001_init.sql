-- +goose Up
-- Migration inicial: schema completo do Streamedia.
-- Recria o conteúdo do antigo internal/db/schema.go como um passo
-- versionado e rastreável pelo goose (tabela goose_db_version).
-- project_id já nasce nas tabelas videos e upload_tokens (não mais
-- adicionado via ALTER TABLE/ensureColumn — T48 tornou-o obrigatório).

CREATE TABLE videos (
    video_id            TEXT PRIMARY KEY,
    status              TEXT NOT NULL DEFAULT 'pending_upload',
    declared_size_bytes INTEGER,
    actual_size_bytes   INTEGER,
    duration_s          INTEGER,
    resolutions         TEXT,
    transcode_attempts  INTEGER NOT NULL DEFAULT 0,
    last_chunk_at       DATETIME,
    error_message       TEXT,
    project_id          INTEGER REFERENCES projects(id),
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE upload_tokens (
    token       TEXT PRIMARY KEY,
    video_id    TEXT NOT NULL UNIQUE,
    project_id  INTEGER REFERENCES projects(id),
    expires_at  DATETIME NOT NULL,
    FOREIGN KEY (video_id) REFERENCES videos(video_id)
);

CREATE TABLE webhook_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id   TEXT NOT NULL,
    event      TEXT NOT NULL,
    payload    TEXT,
    sent_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    success    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE playback_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id     TEXT NOT NULL,
    event_type   TEXT NOT NULL,
    resolution   INTEGER,
    user_agent   TEXT,
    os_family    TEXT,
    occurred_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE projects (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    root_dir        TEXT NOT NULL,
    master_key_hash TEXT NOT NULL,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE video_renditions (
    video_id      TEXT    NOT NULL REFERENCES videos(video_id),
    resolution    INTEGER NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    segment_count INTEGER NOT NULL DEFAULT 0,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (video_id, resolution)
);

CREATE INDEX idx_videos_status ON videos(status);
CREATE INDEX idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX idx_videos_project ON videos(project_id);
CREATE INDEX idx_tokens_expires ON upload_tokens(expires_at);
CREATE INDEX idx_upload_tokens_project ON upload_tokens(project_id);
CREATE INDEX idx_playback_events_video ON playback_events(video_id);
CREATE INDEX idx_playback_events_occurred ON playback_events(occurred_at);
CREATE INDEX idx_projects_slug ON projects(slug);
CREATE INDEX idx_video_renditions_video ON video_renditions(video_id);

-- +goose StatementBegin
CREATE TRIGGER videos_updated_at
AFTER UPDATE ON videos
FOR EACH ROW
BEGIN
    UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = NEW.video_id;
END;
-- +goose StatementEnd

-- +goose Down
-- Rollback: desfaz tudo na ordem inversa, respeitando foreign keys
-- e removendo entidades dependentes antes das independentes.

DROP TRIGGER IF EXISTS videos_updated_at;
DROP INDEX IF EXISTS idx_video_renditions_video;
DROP INDEX IF EXISTS idx_projects_slug;
DROP INDEX IF EXISTS idx_playback_events_occurred;
DROP INDEX IF EXISTS idx_playback_events_video;
DROP INDEX IF EXISTS idx_upload_tokens_project;
DROP INDEX IF EXISTS idx_tokens_expires;
DROP INDEX IF EXISTS idx_videos_project;
DROP INDEX IF EXISTS idx_videos_last_chunk;
DROP INDEX IF EXISTS idx_videos_status;
DROP TABLE IF EXISTS video_renditions;
DROP TABLE IF EXISTS playback_events;
DROP TABLE IF EXISTS upload_tokens;
DROP TABLE IF EXISTS videos;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS webhook_log;
