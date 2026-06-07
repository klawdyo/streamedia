package models

import (
	"testing"

	_ "modernc.org/sqlite"
)

// TestVideoRenditions_PersistsSizeAndSegmentCount verifica que
// UpsertVideoRendition grava tamanho e contagem de segmentos por variante,
// e que reprocessar a mesma variante (re-transcodificação) substitui o
// registro em vez de duplicá-lo — issue #5, T36.
func TestVideoRenditions_PersistsSizeAndSegmentCount(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-renditions", 1000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}

	if err := UpsertVideoRendition(database, "v-renditions", 480, 1_000_000, 10); err != nil {
		t.Fatalf("UpsertVideoRendition (480p): %v", err)
	}
	if err := UpsertVideoRendition(database, "v-renditions", 720, 2_500_000, 10); err != nil {
		t.Fatalf("UpsertVideoRendition (720p): %v", err)
	}

	renditions, err := StorageByVideo(database, "v-renditions")
	if err != nil {
		t.Fatalf("StorageByVideo: %v", err)
	}
	if len(renditions) != 2 {
		t.Fatalf("esperava 2 variantes, obteve %d", len(renditions))
	}
	if renditions[0].Resolution != 480 || renditions[0].SizeBytes != 1_000_000 || renditions[0].SegmentCount != 10 {
		t.Errorf("variante 480p inesperada: %+v", renditions[0])
	}
	if renditions[1].Resolution != 720 || renditions[1].SizeBytes != 2_500_000 || renditions[1].SegmentCount != 10 {
		t.Errorf("variante 720p inesperada: %+v", renditions[1])
	}

	// Re-transcodificação: novo tamanho/contagem para a mesma resolução —
	// deve SUBSTITUIR a linha existente, não duplicar.
	if err := UpsertVideoRendition(database, "v-renditions", 480, 1_200_000, 12); err != nil {
		t.Fatalf("UpsertVideoRendition (480p, retranscode): %v", err)
	}

	renditionsAfter, err := StorageByVideo(database, "v-renditions")
	if err != nil {
		t.Fatalf("StorageByVideo (após retranscode): %v", err)
	}
	if len(renditionsAfter) != 2 {
		t.Fatalf("esperava ainda 2 variantes após retranscodificação (sem duplicar), obteve %d", len(renditionsAfter))
	}
	if renditionsAfter[0].SizeBytes != 1_200_000 || renditionsAfter[0].SegmentCount != 12 {
		t.Errorf("esperava que a variante 480p refletisse os novos valores, obteve %+v", renditionsAfter[0])
	}
}

// TestTotalStorageBytes_SumsOriginalsAndRenditions verifica que
// TotalStorageBytes soma o tamanho dos arquivos originais
// (videos.actual_size_bytes) com o tamanho de todas as variantes geradas
// (video_renditions.size_bytes) — issue #5, T36.
func TestTotalStorageBytes_SumsOriginalsAndRenditions(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-storage-1", 1000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	if err := InsertVideo(database, "v-storage-2", 2000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}

	for _, id := range []string{"v-storage-1", "v-storage-2"} {
		if err := UpdateStatus(database, id, StatusUploading); err != nil {
			t.Fatalf("UpdateStatus(%s, uploading): %v", id, err)
		}
	}

	if err := SetUploadComplete(database, "v-storage-1", 10_000_000); err != nil {
		t.Fatalf("SetUploadComplete v-storage-1: %v", err)
	}
	if err := SetUploadComplete(database, "v-storage-2", 20_000_000); err != nil {
		t.Fatalf("SetUploadComplete v-storage-2: %v", err)
	}

	if err := UpsertVideoRendition(database, "v-storage-1", 480, 1_000_000, 5); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}
	if err := UpsertVideoRendition(database, "v-storage-1", 720, 3_000_000, 5); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}
	if err := UpsertVideoRendition(database, "v-storage-2", 480, 2_000_000, 8); err != nil {
		t.Fatalf("UpsertVideoRendition: %v", err)
	}

	total, err := TotalStorageBytes(database)
	if err != nil {
		t.Fatalf("TotalStorageBytes: %v", err)
	}

	// originais: 10_000_000 + 20_000_000 = 30_000_000
	// variantes:  1_000_000 +  3_000_000 + 2_000_000 = 6_000_000
	const expected = 30_000_000 + 6_000_000
	if total != expected {
		t.Errorf("esperava total de %d bytes, obteve %d", expected, total)
	}
}

// TestTotalDurationSeconds_SumsAcrossVideos verifica que
// TotalDurationSeconds soma a duração de todos os vídeos cadastrados —
// issue #5, T36 ("quantos minutos de vídeo estão armazenados ao todo").
func TestTotalDurationSeconds_SumsAcrossVideos(t *testing.T) {
	database := abreDB(t)

	if err := InsertVideo(database, "v-dur-1", 1000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	if err := InsertVideo(database, "v-dur-2", 2000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	if err := InsertVideo(database, "v-dur-3", 3000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}

	// Leva os vídeos até 'transcoding' (transição válida) para poder chamar SetReady.
	for _, id := range []string{"v-dur-1", "v-dur-2", "v-dur-3"} {
		if err := UpdateStatus(database, id, StatusUploading); err != nil {
			t.Fatalf("UpdateStatus(%s, uploading): %v", id, err)
		}
		if err := UpdateStatus(database, id, StatusUploadComplete); err != nil {
			t.Fatalf("UpdateStatus(%s, upload_complete): %v", id, err)
		}
		if err := UpdateStatus(database, id, StatusTranscoding); err != nil {
			t.Fatalf("UpdateStatus(%s, transcoding): %v", id, err)
		}
	}

	if err := SetReady(database, "v-dur-1", 120, []int{480}); err != nil {
		t.Fatalf("SetReady v-dur-1: %v", err)
	}
	if err := SetReady(database, "v-dur-2", 300, []int{480, 720}); err != nil {
		t.Fatalf("SetReady v-dur-2: %v", err)
	}
	// v-dur-3 permanece sem duração (NULL) — não deve quebrar a soma (COALESCE).

	total, err := TotalDurationSeconds(database)
	if err != nil {
		t.Fatalf("TotalDurationSeconds: %v", err)
	}
	const expected = 120 + 300
	if total != expected {
		t.Errorf("esperava duração total de %d segundos, obteve %d", expected, total)
	}
}

// TestCountVideosByStatus_GroupsCorrectly cria vídeos em diferentes
// estados (pending_upload, transcoding, ready, failed_transcode) e confere
// as contagens por status — issue #5, T36 ("quantos arquivos estão
// pendentes/em processamento/prontos").
func TestCountVideosByStatus_GroupsCorrectly(t *testing.T) {
	database := abreDB(t)

	// pending_upload: fica como está após InsertVideo.
	if err := InsertVideo(database, "v-status-pending", 100); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}

	// transcoding: percorre as transições válidas.
	if err := InsertVideo(database, "v-status-transcoding", 100); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	for _, st := range []VideoStatus{StatusUploading, StatusUploadComplete, StatusTranscoding} {
		if err := UpdateStatus(database, "v-status-transcoding", st); err != nil {
			t.Fatalf("UpdateStatus(v-status-transcoding, %s): %v", st, err)
		}
	}

	// ready: percorre até o fim e chama SetReady.
	if err := InsertVideo(database, "v-status-ready", 100); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	for _, st := range []VideoStatus{StatusUploading, StatusUploadComplete, StatusTranscoding} {
		if err := UpdateStatus(database, "v-status-ready", st); err != nil {
			t.Fatalf("UpdateStatus(v-status-ready, %s): %v", st, err)
		}
	}
	if err := SetReady(database, "v-status-ready", 60, []int{480}); err != nil {
		t.Fatalf("SetReady: %v", err)
	}

	// failed_transcode: usa UpdateStatusWithError, como o worker faz.
	if err := InsertVideo(database, "v-status-failed-1", 100); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	if err := InsertVideo(database, "v-status-failed-2", 100); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	for _, id := range []string{"v-status-failed-1", "v-status-failed-2"} {
		for _, st := range []VideoStatus{StatusUploading, StatusUploadComplete, StatusTranscoding} {
			if err := UpdateStatus(database, id, st); err != nil {
				t.Fatalf("UpdateStatus(%s, %s): %v", id, st, err)
			}
		}
		if err := UpdateStatusWithError(database, id, StatusFailedTranscode, "falha simulada"); err != nil {
			t.Fatalf("UpdateStatusWithError(%s): %v", id, err)
		}
	}

	counts, err := CountVideosByStatus(database)
	if err != nil {
		t.Fatalf("CountVideosByStatus: %v", err)
	}

	expected := map[VideoStatus]int{
		StatusPendingUpload:   1,
		StatusTranscoding:     1,
		StatusReady:           1,
		StatusFailedTranscode: 2,
	}
	for status, want := range expected {
		if got := counts[status]; got != want {
			t.Errorf("status %q: esperava %d vídeo(s), obteve %d (mapa completo: %+v)", status, want, got, counts)
		}
	}

	var total int
	for _, n := range counts {
		total += n
	}
	if total != 5 {
		t.Errorf("esperava 5 vídeos no total entre todos os status, obteve %d (mapa: %+v)", total, counts)
	}
}
