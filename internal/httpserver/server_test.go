package httpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lune/internal/store"
)

func newReadyzTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "readyz.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cache := store.NewRoutingCache(st)
	return New(st, cache, t.TempDir(), "", t.TempDir(), nil, nil), st
}

func TestReadyzRejectsEnabledCpaAccountWithDisabledService(t *testing.T) {
	srv, st := newReadyzTestServer(t)
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:8317",
		APIKey:  "key",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := st.CreateAccount(&store.Account{
		Label:                 "CPA Account",
		SourceKind:            "cpa",
		CpaServiceID:          &svcID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-a",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	srv.cache.Invalidate()

	rr := httptest.NewRecorder()
	srv.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "service disabled") {
		t.Fatalf("expected disabled service reason, got %s", rr.Body.String())
	}
}

func TestReadyzRejectsEnabledCpaAccountWithUnhealthyService(t *testing.T) {
	srv, st := newReadyzTestServer(t)
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:8317",
		APIKey:  "key",
		Enabled: true,
		Status:  "error",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := st.CreateAccount(&store.Account{
		Label:                 "CPA Account",
		SourceKind:            "cpa",
		CpaServiceID:          &svcID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-a",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	}); err != nil {
		t.Fatalf("create account: %v", err)
	}
	srv.cache.Invalidate()

	rr := httptest.NewRecorder()
	srv.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "cpa runtime unavailable") {
		t.Fatalf("expected cpa unavailable reason, got %s", rr.Body.String())
	}
}

func TestReadyzRejectsEnabledCpaAccountMissingServiceEvenWithOtherHealthyService(t *testing.T) {
	srv, st := newReadyzTestServer(t)
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:8317",
		APIKey:  "key",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if err := st.UpdateCpaServiceHealth(svcID, "healthy", ""); err != nil {
		t.Fatalf("mark service healthy: %v", err)
	}
	if _, err := st.CreateAccount(&store.Account{
		Label:                 "Healthy CPA Account",
		SourceKind:            "cpa",
		CpaServiceID:          &svcID,
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-healthy",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	}); err != nil {
		t.Fatalf("create healthy account: %v", err)
	}
	if _, err := st.CreateAccount(&store.Account{
		Label:                 "Missing Service CPA Account",
		SourceKind:            "cpa",
		CpaProvider:           "codex",
		CpaAccountKey:         "codex-missing",
		CpaCredentialStatus:   "ok",
		CpaSubscriptionStatus: "active",
		Enabled:               true,
	}); err != nil {
		t.Fatalf("create missing-service account: %v", err)
	}
	srv.cache.Invalidate()

	rr := httptest.NewRecorder()
	srv.handleReadyz(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "service missing") {
		t.Fatalf("expected missing service reason, got %s", rr.Body.String())
	}
}
