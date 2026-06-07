package models

import (
	"database/sql"
	"testing"

	"github.com/klawdyo/streamedia/internal/db"
)

// abreDBStats abre banco em memória para testes de estatísticas.
// (Função separada para evitar conflito de redefinição com outros _test.go)
func abreDBStats(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco de teste: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestRecordPlaybackEvent_Inserts(t *testing.T) {
	// Verifica que RecordEvent insere um registro com os campos corretos
	// e que o os_family é detectado corretamente para um user-agent iOS.
	database := abreDBStats(t)
	res480 := 480

	err := RecordEvent(database, "vid1", "playback", &res480, "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	if err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}

	var videoID, eventType, osFamily string
	var resolution int
	row := database.QueryRow(`SELECT video_id, event_type, resolution, os_family FROM playback_events WHERE video_id = ?`, "vid1")
	if err := row.Scan(&videoID, &eventType, &resolution, &osFamily); err != nil {
		t.Fatalf("erro ao buscar evento inserido: %v", err)
	}

	if videoID != "vid1" || eventType != "playback" || resolution != 480 {
		t.Errorf("registro inesperado: video_id=%s event_type=%s resolution=%d", videoID, eventType, resolution)
	}
	if osFamily != "ios" {
		t.Errorf("os_family = %q, esperava \"ios\"", osFamily)
	}
}

func TestRecordEvent_NilResolution(t *testing.T) {
	// Verifica que RecordEvent com resolution nil grava NULL no banco
	// e que o os_family é detectado corretamente para um user-agent Windows.
	database := abreDBStats(t)

	err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	if err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}

	var resolution sql.NullInt64
	var osFamily string
	row := database.QueryRow(`SELECT resolution, os_family FROM playback_events WHERE video_id = ?`, "vid1")
	if err := row.Scan(&resolution, &osFamily); err != nil {
		t.Fatalf("erro ao buscar evento inserido: %v", err)
	}

	if resolution.Valid {
		t.Errorf("esperava resolution NULL, obteve %d", resolution.Int64)
	}
	if osFamily != "windows" {
		t.Errorf("os_family = %q, esperava \"windows\"", osFamily)
	}
}

func TestDetectOSFamily_VariousUserAgents(t *testing.T) {
	// Testa strings de UA representativas dos principais SOs e o fallback "other".
	cases := []struct {
		userAgent string
		expected  string
	}{
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)", "ios"},
		{"Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)", "ios"},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8)", "android"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "windows"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "macos"},
		{"Mozilla/5.0 (X11; Linux x86_64)", "linux"},
		{"", "other"},
		{"algum-user-agent-totalmente-desconhecido/1.0", "other"},
	}

	for _, c := range cases {
		got := detectOSFamily(c.userAgent)
		if got != c.expected {
			t.Errorf("detectOSFamily(%q) = %q, esperava %q", c.userAgent, got, c.expected)
		}
	}
}

func TestCountEventsByType(t *testing.T) {
	// Insere eventos de tipos variados e verifica a contagem por tipo.
	database := abreDBStats(t)
	res720 := 720

	mustRecord := func(videoID, eventType string, resolution *int) {
		if err := RecordEvent(database, videoID, eventType, resolution, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
			t.Fatalf("RecordEvent falhou: %v", err)
		}
	}

	mustRecord("vid1", "playback", nil)
	mustRecord("vid1", "playback", nil)
	mustRecord("vid1", "download_segment", &res720)
	mustRecord("vid2", "upload_complete", nil)

	count, err := CountEventsByType(database, "playback")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 2 {
		t.Errorf("CountEventsByType(playback) = %d, esperava 2", count)
	}

	count, err = CountEventsByType(database, "upload_complete")
	if err != nil {
		t.Fatalf("CountEventsByType falhou: %v", err)
	}
	if count != 1 {
		t.Errorf("CountEventsByType(upload_complete) = %d, esperava 1", count)
	}
}

func TestAggregateByResolution(t *testing.T) {
	// Insere eventos com resoluções variadas (incluindo NULL) e verifica
	// que AggregateByResolution conta corretamente por resolução, ignorando NULL.
	database := abreDBStats(t)
	res480 := 480
	res720 := 720

	mustRecord := func(videoID string, resolution *int) {
		if err := RecordEvent(database, videoID, "download_segment", resolution, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
			t.Fatalf("RecordEvent falhou: %v", err)
		}
	}

	mustRecord("vid1", &res480)
	mustRecord("vid1", &res480)
	mustRecord("vid1", &res720)
	mustRecord("vid1", nil)
	mustRecord("vid2", &res720) // outro vídeo, não deve contar

	result, err := AggregateByResolution(database, "vid1")
	if err != nil {
		t.Fatalf("AggregateByResolution falhou: %v", err)
	}

	if result[480] != 2 {
		t.Errorf("result[480] = %d, esperava 2", result[480])
	}
	if result[720] != 1 {
		t.Errorf("result[720] = %d, esperava 1", result[720])
	}
	if _, ok := result[0]; ok {
		t.Errorf("não esperava entrada para resolução NULL/zero")
	}
}

func TestAggregateByOS(t *testing.T) {
	// Insere eventos com os_family variados e verifica a contagem global por SO.
	database := abreDBStats(t)

	mustRecord := func(videoID, userAgent string) {
		if err := RecordEvent(database, videoID, "playback", nil, userAgent); err != nil {
			t.Fatalf("RecordEvent falhou: %v", err)
		}
	}

	mustRecord("vid1", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	mustRecord("vid1", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
	mustRecord("vid2", "Mozilla/5.0 (Linux; Android 14; Pixel 8)")
	mustRecord("vid2", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	result, err := AggregateByOS(database)
	if err != nil {
		t.Fatalf("AggregateByOS falhou: %v", err)
	}

	if result["ios"] != 2 {
		t.Errorf("result[ios] = %d, esperava 2", result["ios"])
	}
	if result["android"] != 1 {
		t.Errorf("result[android] = %d, esperava 1", result["android"])
	}
	if result["windows"] != 1 {
		t.Errorf("result[windows] = %d, esperava 1", result["windows"])
	}
}

func TestAggregateByDayOfWeek(t *testing.T) {
	// Insere eventos com occurred_at em dias da semana conhecidos e verifica
	// que AggregateByDayOfWeek conta corretamente via strftime('%w', ...).
	// 0=domingo .. 6=sábado. 2026-06-07 é um domingo; 2026-06-08 é uma segunda-feira.
	database := abreDBStats(t)

	if err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}
	if err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}
	if err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}

	// Sobrescreve occurred_at com datas de dias da semana conhecidos,
	// normalizando com datetime() para evitar o bug RFC3339 vs formato SQLite.
	if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime('2026-06-07 10:00:00') WHERE id = 1`); err != nil {
		t.Fatalf("erro ao ajustar occurred_at: %v", err)
	}
	if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime('2026-06-08 10:00:00') WHERE id = 2`); err != nil {
		t.Fatalf("erro ao ajustar occurred_at: %v", err)
	}
	if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime('2026-06-08 11:00:00') WHERE id = 3`); err != nil {
		t.Fatalf("erro ao ajustar occurred_at: %v", err)
	}

	result, err := AggregateByDayOfWeek(database)
	if err != nil {
		t.Fatalf("AggregateByDayOfWeek falhou: %v", err)
	}

	if result[0] != 1 {
		t.Errorf("result[0] (domingo) = %d, esperava 1", result[0])
	}
	if result[1] != 2 {
		t.Errorf("result[1] (segunda) = %d, esperava 2", result[1])
	}
}
