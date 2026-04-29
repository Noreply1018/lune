package router

import (
	"errors"
	"path/filepath"
	"testing"

	"lune/internal/store"
)

func TestForceAccountCanProbeUnhealthyAccountInTokenPool(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Probe Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "recovering-account",
		SourceKind: "openai_compat",
		BaseURL:    "http://example.invalid/v1",
		APIKey:     "sk-upstream",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "previous failure"); err != nil {
		t.Fatalf("UpdateAccountHealth: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-probe"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	if _, err := rt.Resolve("gpt-probe", &poolID, nil); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected normal routing to reject unhealthy account, got %v", err)
	}

	resolved, err := rt.Resolve("gpt-probe", &poolID, &accountID)
	if err != nil {
		t.Fatalf("expected forced account probe route, got %v", err)
	}
	if resolved.AccountID != accountID || resolved.PoolID != poolID {
		t.Fatalf("unexpected route: %+v", resolved)
	}
}
