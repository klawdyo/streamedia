package transcode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThumbnailScale(t *testing.T) {
	cases := []struct {
		name                  string
		originW, originH, res int
		wantW, wantH          int
	}{
		// Landscape 16:9 — a altura é o "p".
		{"16:9 480p", 1920, 1080, 480, 854, 480},
		{"16:9 720p", 1920, 1080, 720, 1280, 720},
		{"16:9 1080p", 1920, 1080, 1080, 1920, 1080},
		// Portrait 9:16 — a largura é o "p".
		{"9:16 480p", 1080, 1920, 480, 480, 854},
		// Quadrado 1:1.
		{"1:1 480p", 1000, 1000, 480, 480, 480},
		// 4:3 landscape.
		{"4:3 480p", 640, 480, 480, 640, 480},
		// Dimensões inválidas (ffprobe falhou) → assume 16:9.
		{"sem dimensões", 0, 0, 480, 854, 480},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h := thumbnailScale(tc.originW, tc.originH, tc.res)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("thumbnailScale(%d,%d,%d) = %dx%d; esperado %dx%d",
					tc.originW, tc.originH, tc.res, w, h, tc.wantW, tc.wantH)
			}
			// Dimensões sempre pares.
			if w%2 != 0 || h%2 != 0 {
				t.Fatalf("dimensões devem ser pares: %dx%d", w, h)
			}
		})
	}
}

func TestBuildThumbnailArgs(t *testing.T) {
	args := buildThumbnailArgs("/in/video", "/out/thumb_480.jpg", 854, 480, 1)
	joined := strings.Join(args, " ")

	// Deve buscar 1 frame, com seek em 1s ANTES do -i (input seeking).
	wantSubs := []string{
		"-ss 1",
		"-i /in/video",
		"-frames:v 1",
		"-vf scale=854:480",
		"-q:v " + jpegQScale,
		"-f image2",
		"/out/thumb_480.jpg",
	}
	for _, sub := range wantSubs {
		if !strings.Contains(joined, sub) {
			t.Fatalf("args não contêm %q: %v", sub, joined)
		}
	}

	// O -ss precisa vir antes do -i (input seeking, e não output seeking).
	ssIdx, iIdx := indexOf(args, "-ss"), indexOf(args, "-i")
	if ssIdx == -1 || iIdx == -1 || ssIdx > iIdx {
		t.Fatalf("esperado -ss antes de -i (input seeking): %v", args)
	}
}

func TestGenerateThumbnails_CreatesOnePerResolution(t *testing.T) {
	outputDir := t.TempDir()

	// Mock de FFmpeg que cria o arquivo de saída (último argumento) — simula a
	// gravação do JPEG pelo ffmpeg real.
	var calls [][]string
	mock := &mockFFmpeg{
		createFiles: func(args []string) {
			out := args[len(args)-1]
			calls = append(calls, args)
			_ = os.WriteFile(out, []byte("jpeg"), 0644)
		},
	}
	w := &Worker{ffmpeg: mock}

	w.generateThumbnails("vid-1", "/in/video", outputDir, 1920, 1080, []int{480, 720})

	for _, res := range []int{480, 720} {
		path := filepath.Join(outputDir, "thumb_"+itoa(res)+".jpg")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("esperado thumbnail %dp criado em %s: %v", res, path, err)
		}
	}
	// Como o mock sempre sucede, deve ter havido exatamente 1 chamada por
	// resolução (sem o retry de fallback -ss 0).
	if len(calls) != 2 {
		t.Fatalf("esperado 2 chamadas ao ffmpeg (uma por resolução), obtido %d", len(calls))
	}
}

func TestGenerateThumbnails_FallsBackToFirstFrame(t *testing.T) {
	outputDir := t.TempDir()

	// Mock que falha quando o seek é 1s e sucede quando é 0s — exercita o
	// fallback para o primeiro frame.
	var seeks []string
	mock := &mockFFmpeg{
		runFunc: func(args []string) error {
			seek := args[indexOf(args, "-ss")+1]
			seeks = append(seeks, seek)
			if seek == "1" {
				return context.DeadlineExceeded // simula "sem frame em 1s"
			}
			_ = os.WriteFile(args[len(args)-1], []byte("jpeg"), 0644)
			return nil
		},
	}
	w := &Worker{ffmpeg: mock}

	w.generateThumbnails("vid-1", "/in/video", outputDir, 1920, 1080, []int{480})

	if _, err := os.Stat(filepath.Join(outputDir, "thumb_480.jpg")); err != nil {
		t.Fatalf("esperado thumbnail criado pelo fallback: %v", err)
	}
	// Deve ter tentado 1s e depois 0s.
	if len(seeks) != 2 || seeks[0] != "1" || seeks[1] != "0" {
		t.Fatalf("esperado seeks [1 0], obtido %v", seeks)
	}
}

// indexOf devolve o índice da primeira ocorrência de s em args, ou -1.
func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}

// itoa é um helper local para evitar importar strconv só nos testes.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
