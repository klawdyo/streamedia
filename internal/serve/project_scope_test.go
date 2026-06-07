package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/klawdyo/streamedia/internal/auth"
)

// secondVideoID é um segundo UUID v4 válido fixo, distinto de testVideoID,
// usado para testar que tokens de reprodução são escopados a um único vídeo.
const secondVideoID = "b1ffcd88-8d1a-4d59-9c7c-7cc0ce491b22"

// TestPlayToken_ScopedToSingleVideo verifica que um token de reprodução
// gerado para um vídeo não autentica o acesso a outro — mesmo que ambos
// pertençam ao mesmo projeto (issue #6, T33: "um token de um vídeo não
// serve para outro"). A vinculação ao vídeo já vem do payload assinado
// "{video_id}:{expires}" (auth.GeneratePlayToken/ValidatePlayToken); o
// vínculo de projeto é transitivo — o vídeo carrega project_id, então um
// token só abre o vídeo (e, por extensão, o projeto) ao qual foi emitido.
func TestPlayToken_ScopedToSingleVideo(t *testing.T) {
	cfg := newTestConfig(t)
	database := newTestDB(t)
	insertVideo(t, database, testVideoID, "ready")
	insertVideo(t, database, secondVideoID, "ready")

	const m3u8Content = "#EXTM3U\n#EXT-X-VERSION:3\n"
	writeFile(t, filepath.Join(cfg.MediaDir, testVideoID, "master.m3u8"), m3u8Content)
	writeFile(t, filepath.Join(cfg.MediaDir, secondVideoID, "master.m3u8"), m3u8Content)

	expires := time.Now().Add(time.Hour).Unix()
	tokenForFirst := auth.GeneratePlayToken(testSecret, testVideoID, expires)

	h := NewMasterHandler(cfg, database)

	// O token emitido para testVideoID abre normalmente o seu próprio vídeo...
	req := httptest.NewRequest(http.MethodGet,
		"/videos/"+testVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+tokenForFirst, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token deveria autenticar o próprio vídeo: esperado 200, obtido %d", rec.Code)
	}

	// ...mas é rejeitado ao tentar acessar outro vídeo (mesmo válido e "ready").
	reqOther := httptest.NewRequest(http.MethodGet,
		"/videos/"+secondVideoID+"/master.m3u8?expires="+itoa(expires)+"&token="+tokenForFirst, nil)
	recOther := httptest.NewRecorder()
	h.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusUnauthorized {
		t.Fatalf("token de um vídeo não deveria autenticar outro: esperado 401, obtido %d", recOther.Code)
	}
}
