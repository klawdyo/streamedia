package notify

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/klawdyo/streamedia/internal/db"
	"github.com/klawdyo/streamedia/internal/models"
)

const testVideoID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// captureSink coleta as notificações entregues, sinalizando por canal.
type captureSink struct {
	ch chan Notification
}

func (c *captureSink) Deliver(n Notification) { c.ch <- n }

// TestBuild_PopulatesFields verifica a montagem da Notification a partir do vídeo.
func TestBuild_PopulatesFields(t *testing.T) {
	v := &models.Video{
		Status:       models.StatusReady,
		DurationS:    12,
		Resolutions:  []int{480, 720},
		ErrorMessage: "",
	}
	n := Build("vid-1", "ready", v)

	if n.VideoID != "vid-1" || n.Event != "ready" {
		t.Fatalf("VideoID/Event: %+v", n)
	}
	if n.Status != "ready" {
		t.Errorf("Status: esperado ready, obtido %q", n.Status)
	}
	if n.DurationS == nil || *n.DurationS != 12 {
		t.Errorf("DurationS: esperado *12, obtido %v", n.DurationS)
	}
	if len(n.Resolutions) != 2 {
		t.Errorf("Resolutions: esperado [480 720], obtido %v", n.Resolutions)
	}
	if n.ErrorMessage != nil {
		t.Errorf("ErrorMessage: esperado nil, obtido %v", *n.ErrorMessage)
	}
}

// TestBuild_ErrorMessageAndEmptyResolutions cobre os ramos opcionais.
func TestBuild_ErrorMessageAndEmptyResolutions(t *testing.T) {
	v := &models.Video{Status: models.StatusFailedTranscode, ErrorMessage: "boom"}
	n := Build("vid-2", "failed", v)
	if n.ErrorMessage == nil || *n.ErrorMessage != "boom" {
		t.Errorf("ErrorMessage: esperado *boom, obtido %v", n.ErrorMessage)
	}
	if n.Resolutions == nil {
		t.Error("Resolutions nunca deve ser nil (esperado []int{})")
	}
	if n.DurationS != nil {
		t.Errorf("DurationS: esperado nil para duração 0, obtido %v", *n.DurationS)
	}
}

// TestNotify_FansOutToAllSinks confirma que cada sink recebe a notificação.
func TestNotify_FansOutToAllSinks(t *testing.T) {
	database := openMemDB(t)
	if err := models.InsertVideo(database, testVideoID, 1000); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}

	s1 := &captureSink{ch: make(chan Notification, 1)}
	s2 := &captureSink{ch: make(chan Notification, 1)}
	no := New(database, s1, s2)

	no.Notify(testVideoID, "processing", "")

	for i, s := range []*captureSink{s1, s2} {
		select {
		case n := <-s.ch:
			if n.VideoID != testVideoID || n.Event != "processing" {
				t.Errorf("sink %d: notificação inesperada %+v", i, n)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("sink %d: timeout esperando notificação", i)
		}
	}
}

// TestNotify_MissingVideoNoDeliver garante que um vídeo inexistente não gera
// entrega (e não causa pânico).
func TestNotify_MissingVideoNoDeliver(t *testing.T) {
	database := openMemDB(t)
	s := &captureSink{ch: make(chan Notification, 1)}
	no := New(database, s)

	no.Notify("nao-existe", "ready", "")

	select {
	case n := <-s.ch:
		t.Fatalf("não deveria entregar para vídeo inexistente, recebeu %+v", n)
	case <-time.After(150 * time.Millisecond):
		// esperado: nada entregue
	}
}

// TestNotify_NilNotifierIsNoOp garante que chamar Notify num *Notifier nil não
// causa pânico (defensivo para call-sites sem notifier configurado).
func TestNotify_NilNotifierIsNoOp(t *testing.T) {
	var no *Notifier
	no.Notify("v", "ready", "") // não deve entrar em pânico
}
