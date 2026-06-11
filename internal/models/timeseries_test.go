package models

import (
	"testing"
)

// As séries temporais de upload derivam de videos.created_at; as de playback,
// de playback_events.occurred_at. Em ambos os casos os testes inserem com
// timestamps controlados (via UPDATE após o insert, normalizando com
// datetime() — mesmo cuidado de TestAggregateByDayOfWeek) e conferem os
// buckets. 2026-06-07 é domingo (%w=0); 2026-06-08 é segunda (%w=1).

func TestUploadsByDate(t *testing.T) {
	database := abreDBStats(t)

	mustInsert := func(videoID, when string) {
		if err := InsertVideo(database, videoID, 100); err != nil {
			t.Fatalf("InsertVideo falhou: %v", err)
		}
		if _, err := database.Exec(`UPDATE videos SET created_at = datetime(?) WHERE video_id = ?`, when, videoID); err != nil {
			t.Fatalf("erro ao ajustar created_at: %v", err)
		}
	}

	mustInsert("v1", "2026-06-07 10:00:00")
	mustInsert("v2", "2026-06-07 23:00:00")
	mustInsert("v3", "2026-06-08 11:00:00")

	result, err := UploadsByDate(database)
	if err != nil {
		t.Fatalf("UploadsByDate falhou: %v", err)
	}
	if result["2026-06-07"] != 2 {
		t.Errorf("result[2026-06-07] = %d, esperava 2", result["2026-06-07"])
	}
	if result["2026-06-08"] != 1 {
		t.Errorf("result[2026-06-08] = %d, esperava 1", result["2026-06-08"])
	}
}

func TestUploadsByDayOfWeek(t *testing.T) {
	database := abreDBStats(t)

	mustInsert := func(videoID, when string) {
		if err := InsertVideo(database, videoID, 100); err != nil {
			t.Fatalf("InsertVideo falhou: %v", err)
		}
		if _, err := database.Exec(`UPDATE videos SET created_at = datetime(?) WHERE video_id = ?`, when, videoID); err != nil {
			t.Fatalf("erro ao ajustar created_at: %v", err)
		}
	}

	mustInsert("v1", "2026-06-07 10:00:00") // domingo
	mustInsert("v2", "2026-06-08 10:00:00") // segunda
	mustInsert("v3", "2026-06-08 12:00:00") // segunda

	result, err := UploadsByDayOfWeek(database)
	if err != nil {
		t.Fatalf("UploadsByDayOfWeek falhou: %v", err)
	}
	if result[0] != 1 {
		t.Errorf("result[0] (domingo) = %d, esperava 1", result[0])
	}
	if result[1] != 2 {
		t.Errorf("result[1] (segunda) = %d, esperava 2", result[1])
	}
}

func TestUploadsByHour(t *testing.T) {
	database := abreDBStats(t)

	mustInsert := func(videoID, when string) {
		if err := InsertVideo(database, videoID, 100); err != nil {
			t.Fatalf("InsertVideo falhou: %v", err)
		}
		if _, err := database.Exec(`UPDATE videos SET created_at = datetime(?) WHERE video_id = ?`, when, videoID); err != nil {
			t.Fatalf("erro ao ajustar created_at: %v", err)
		}
	}

	mustInsert("v1", "2026-06-07 10:15:00")
	mustInsert("v2", "2026-06-07 10:45:00")
	mustInsert("v3", "2026-06-08 22:00:00")

	result, err := UploadsByHour(database)
	if err != nil {
		t.Fatalf("UploadsByHour falhou: %v", err)
	}
	if result[10] != 2 {
		t.Errorf("result[10] = %d, esperava 2", result[10])
	}
	if result[22] != 1 {
		t.Errorf("result[22] = %d, esperava 1", result[22])
	}
}

func TestCountVideos(t *testing.T) {
	database := abreDBStats(t)

	if n, err := CountVideos(database); err != nil || n != 0 {
		t.Fatalf("CountVideos vazio = %d (err=%v), esperava 0", n, err)
	}

	for _, id := range []string{"v1", "v2", "v3"} {
		if err := InsertVideo(database, id, 100); err != nil {
			t.Fatalf("InsertVideo falhou: %v", err)
		}
	}

	n, err := CountVideos(database)
	if err != nil {
		t.Fatalf("CountVideos falhou: %v", err)
	}
	if n != 3 {
		t.Errorf("CountVideos = %d, esperava 3", n)
	}
}

func TestPlaybackByDate(t *testing.T) {
	database := abreDBStats(t)

	for i := 0; i < 3; i++ {
		if err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
			t.Fatalf("RecordEvent falhou: %v", err)
		}
	}
	if err := RecordEvent(database, "vid2", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
		t.Fatalf("RecordEvent falhou: %v", err)
	}

	mustSet := func(id int, when string) {
		if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime(?) WHERE id = ?`, when, id); err != nil {
			t.Fatalf("erro ao ajustar occurred_at: %v", err)
		}
	}
	mustSet(1, "2026-06-07 10:00:00")
	mustSet(2, "2026-06-07 11:00:00")
	mustSet(3, "2026-06-08 10:00:00")
	mustSet(4, "2026-06-07 12:00:00") // vid2

	// Global: todos os vídeos.
	all, err := PlaybackByDate(database)
	if err != nil {
		t.Fatalf("PlaybackByDate global falhou: %v", err)
	}
	if all["2026-06-07"] != 3 {
		t.Errorf("global[2026-06-07] = %d, esperava 3", all["2026-06-07"])
	}
	if all["2026-06-08"] != 1 {
		t.Errorf("global[2026-06-08] = %d, esperava 1", all["2026-06-08"])
	}

	// Por vídeo: só vid1.
	v1, err := PlaybackByDate(database, "vid1")
	if err != nil {
		t.Fatalf("PlaybackByDate vid1 falhou: %v", err)
	}
	if v1["2026-06-07"] != 2 {
		t.Errorf("vid1[2026-06-07] = %d, esperava 2", v1["2026-06-07"])
	}
	if v1["2026-06-08"] != 1 {
		t.Errorf("vid1[2026-06-08] = %d, esperava 1", v1["2026-06-08"])
	}
}

func TestPlaybackByHour(t *testing.T) {
	database := abreDBStats(t)

	for i := 0; i < 3; i++ {
		if err := RecordEvent(database, "vid1", "playback", nil, "Mozilla/5.0 (Windows NT 10.0)"); err != nil {
			t.Fatalf("RecordEvent falhou: %v", err)
		}
	}

	mustSet := func(id int, when string) {
		if _, err := database.Exec(`UPDATE playback_events SET occurred_at = datetime(?) WHERE id = ?`, when, id); err != nil {
			t.Fatalf("erro ao ajustar occurred_at: %v", err)
		}
	}
	mustSet(1, "2026-06-07 09:00:00")
	mustSet(2, "2026-06-07 09:30:00")
	mustSet(3, "2026-06-07 14:00:00")

	result, err := PlaybackByHour(database, "vid1")
	if err != nil {
		t.Fatalf("PlaybackByHour falhou: %v", err)
	}
	if result[9] != 2 {
		t.Errorf("result[9] = %d, esperava 2", result[9])
	}
	if result[14] != 1 {
		t.Errorf("result[14] = %d, esperava 1", result[14])
	}
}
