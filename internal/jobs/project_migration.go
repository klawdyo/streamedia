package jobs

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/klawdyo/streamedia/internal/models"
)

// legacyProjectName é o nome do projeto "guarda-chuva" para onde vídeos
// criados antes da issue #6 (sem project_id) são migrados — issue #6, T34,
// estratégia (a) sugerida na tarefa: um único layout simplifica todo o
// código de serving/transcodificação subsequente (sempre há um projeto).
const legacyProjectName = "Legacy"

// MigrateLegacyVideos garante que todo vídeo tenha um projeto associado —
// issue #6, T34. Roda na inicialização (como o startup recovery do T21):
//
//  1. Garante a existência do projeto "Legacy" (slug "legacy"), criado uma
//     única vez — idempotente via GetProjectBySlug/CreateProject.
//  2. Para cada vídeo com project_id NULL: move seu diretório de
//     <MEDIA_DIR>/<video_id> para <MEDIA_DIR>/legacy/<video_id> (se existir
//     e ainda não tiver sido movido) e associa o vídeo ao projeto Legacy.
//
// Idempotente: rodar novamente não duplica nem corrompe nada — vídeos já
// migrados (project_id preenchido) não aparecem mais na consulta, e a
// movimentação de diretório só ocorre se a origem existir e o destino não.
// Retorna o número de vídeos migrados nesta execução.
func MigrateLegacyVideos(db *sql.DB, mediaDir string) (int, error) {
	legacy, err := getOrCreateLegacyProject(db)
	if err != nil {
		return 0, fmt.Errorf("erro ao garantir o projeto Legacy: %w", err)
	}

	rows, err := db.Query(`SELECT video_id FROM videos WHERE project_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("erro ao consultar vídeos sem projeto: %w", err)
	}
	var videoIDs []string
	for rows.Next() {
		var videoID string
		if err := rows.Scan(&videoID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("erro ao ler video_id: %w", err)
		}
		videoIDs = append(videoIDs, videoID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("erro ao iterar vídeos sem projeto: %w", err)
	}
	rows.Close()

	migrated := 0
	for _, videoID := range videoIDs {
		if err := migrateOneLegacyVideo(db, mediaDir, legacy, videoID); err != nil {
			return migrated, fmt.Errorf("erro ao migrar vídeo %s para o projeto Legacy: %w", videoID, err)
		}
		migrated++
	}

	if migrated > 0 {
		log.Printf("[migration] %d vídeo(s) sem projeto migrados para %q (slug=%s)", migrated, legacy.Name, legacy.Slug)
	}
	return migrated, nil
}

// getOrCreateLegacyProject busca o projeto Legacy pelo slug; cria-o na
// primeira execução (CreateProject já resolve colisão de slug e gera a
// chave mestra — descartada aqui, já que este projeto não é operado via
// chave de API, apenas serve de "guarda-chuva" para vídeos antigos).
func getOrCreateLegacyProject(db *sql.DB) (*models.Project, error) {
	project, err := models.GetProjectBySlug(db, models.Slugify(legacyProjectName))
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	project, _, err = models.CreateProject(db, legacyProjectName)
	if err != nil {
		return nil, err
	}
	return project, nil
}

// migrateOneLegacyVideo move o diretório de armazenamento de um vídeo
// (se existir e ainda não tiver sido movido) e associa o vídeo ao projeto
// informado, em uma única transação — garante que a migração de DB e a
// movimentação em disco não fiquem dessincronizadas em caso de erro.
func migrateOneLegacyVideo(db *sql.DB, mediaDir string, project *models.Project, videoID string) error {
	oldDir := filepath.Join(mediaDir, videoID)
	newDir := filepath.Join(mediaDir, project.RootDir, videoID)

	if oldDir != newDir {
		if _, err := os.Stat(oldDir); err == nil {
			if _, err := os.Stat(newDir); os.IsNotExist(err) {
				if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
					return fmt.Errorf("erro ao criar diretório do projeto: %w", err)
				}
				if err := os.Rename(oldDir, newDir); err != nil {
					return fmt.Errorf("erro ao mover diretório de %s para %s: %w", oldDir, newDir, err)
				}
				log.Printf("[migration] %s: %s → %s", videoID, oldDir, newDir)
			}
			// destino já existe (execução anterior interrompida no meio) — não
			// sobrescreve; assume que o conteúdo já foi movido corretamente.
		}
		// origem inexistente (upload nunca terminou, ou já migrado): nada a mover.
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE videos SET project_id = ? WHERE video_id = ?`, project.ID, videoID); err != nil {
		return fmt.Errorf("erro ao associar vídeo ao projeto Legacy: %w", err)
	}
	return tx.Commit()
}
