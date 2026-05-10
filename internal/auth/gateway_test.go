package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"lune/internal/store"
)

func TestGatewayAuthRejectsDisabledToken(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	if _, err := st.CreateToken(&store.AccessToken{
		Name:    "disabled",
		Token:   "sk-disabled",
		PoolID:  &poolID,
		Enabled: false,
	}); err != nil {
		t.Fatalf("create token: %v", err)
	}

	cache := store.NewRoutingCache(st)
	handler := GatewayAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), cache)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer sk-disabled")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled token to be rejected, got %d", rr.Code)
	}
}
