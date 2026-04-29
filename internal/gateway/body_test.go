package gateway

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplayBodySpillsLargeRequestsAndCleansUp(t *testing.T) {
	tmpDir := t.TempDir()
	payload := `{"input":"` + strings.Repeat("x", 2048) + `","model":"gpt-test","stream":true}`
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(payload))

	body, err := NewReplayBody(req, 4096, 128, tmpDir)
	if err != nil {
		t.Fatalf("NewReplayBody: %v", err)
	}
	if body.Storage() != "disk" {
		t.Fatalf("expected disk storage, got %s", body.Storage())
	}
	if _, err := os.Stat(body.path); err != nil {
		t.Fatalf("expected replay file: %v", err)
	}

	env, err := ParseRequestEnvelope(body)
	if err != nil {
		t.Fatalf("ParseRequestEnvelope: %v", err)
	}
	if env.Model != "gpt-test" || !env.Stream {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	path := body.path
	if err := body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected replay file to be removed, stat err=%v", err)
	}
}

func TestReplayBodyRejectsOversizedRequests(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(strings.Repeat("x", 20)))
	if _, err := NewReplayBody(req, 10, 4, t.TempDir()); err != ErrBodyTooLarge {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

func TestCleanupReplayDirRemovesStaleTempFiles(t *testing.T) {
	tmpDir := t.TempDir()
	stale := filepath.Join(tmpDir, "lune-body-stale.tmp")
	keep := filepath.Join(tmpDir, "keep.txt")
	if err := os.WriteFile(stale, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CleanupReplayDir(tmpDir); err != nil {
		t.Fatalf("CleanupReplayDir: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("expected stale tmp removed, stat err=%v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("expected non-temp file kept: %v", err)
	}
}
