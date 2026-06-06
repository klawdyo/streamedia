package db

// schema define o DDL completo do banco de dados.
// Todas as declarações usam IF NOT EXISTS para ser idempotentes.
const schema = `
CREATE TABLE IF NOT EXISTS videos (
  video_id            TEXT PRIMARY KEY,
  status              TEXT NOT NULL DEFAULT 'pending_upload',
  declared_size_bytes INTEGER,
  actual_size_bytes   INTEGER,
  duration_s          INTEGER,
  resolutions         TEXT,
  transcode_attempts  INTEGER NOT NULL DEFAULT 0,
  last_chunk_at       DATETIME,
  error_message       TEXT,
  created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upload_tokens (
  token      TEXT PRIMARY KEY,
  video_id   TEXT NOT NULL UNIQUE,
  expires_at DATETIME NOT NULL,
  FOREIGN KEY (video_id) REFERENCES videos(video_id)
);

CREATE TABLE IF NOT EXISTS webhook_log (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  video_id   TEXT NOT NULL,
  event      TEXT NOT NULL,
  payload    TEXT,
  sent_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  success    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status);
CREATE INDEX IF NOT EXISTS idx_videos_last_chunk ON videos(last_chunk_at);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON upload_tokens(expires_at);

CREATE TRIGGER IF NOT EXISTS videos_updated_at
AFTER UPDATE ON videos
FOR EACH ROW
BEGIN
  UPDATE videos SET updated_at = CURRENT_TIMESTAMP WHERE video_id = NEW.video_id;
END;
`
