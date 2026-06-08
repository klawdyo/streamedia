-- +goose Up
-- Garante que o projeto padrão "Default" sempre existe (issue #10, T48).
-- A chave mestra é gerada deterministicamente via SHA-256 de randomblob(64)
-- do próprio SQLite — sem dependência de código Go externo.
-- INSERT OR IGNORE: idempotente, não duplica se o projeto já existir.

INSERT OR IGNORE INTO projects (name, slug, root_dir, master_key_hash)
VALUES ('Default', 'default', 'default', hex(randomblob(32)));

-- +goose Down
-- Não remove o projeto default — ele pode ter vídeos associados.
