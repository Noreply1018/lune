package router

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"lune/internal/store"
)

func TestForceAccountCannotBypassUnroutableAccount(t *testing.T) {
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

	if _, err := rt.Resolve("gpt-probe", &poolID, &accountID); !errors.Is(err, ErrNoHealthyAccount) {
		t.Fatalf("expected forced routing to reject unhealthy account, got %v", err)
	}
}

func TestRoutingSkipsAccountInServingCooldown(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	firstID, err := st.CreateAccount(&store.Account{
		Label:      "cooldown",
		SourceKind: "openai_compat",
		BaseURL:    "http://first.example/v1",
		APIKey:     "sk-first",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount first: %v", err)
	}
	secondID, err := st.CreateAccount(&store.Account{
		Label:      "healthy",
		SourceKind: "openai_compat",
		BaseURL:    "http://second.example/v1",
		APIKey:     "sk-second",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount second: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, firstID); err != nil {
		t.Fatalf("AddPoolMember first: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, secondID); err != nil {
		t.Fatalf("AddPoolMember second: %v", err)
	}
	if err := st.RefreshAccountModels(firstID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels first: %v", err)
	}
	if err := st.RefreshAccountModels(secondID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels second: %v", err)
	}
	if err := st.MarkAccountServingFailure(firstID, "EOF", time.Now().Add(5*time.Minute)); err != nil {
		t.Fatalf("MarkAccountServingFailure: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected route to second account, got %v", err)
	}
	if resolved.AccountID != secondID {
		t.Fatalf("expected second account, got %d", resolved.AccountID)
	}
}

func TestRoutingAllowsAccountAfterServingCooldownExpires(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	poolID, err := st.CreatePool("Pool", 0, true)
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	accountID, err := st.CreateAccount(&store.Account{
		Label:      "expired-cooldown",
		SourceKind: "openai_compat",
		BaseURL:    "http://example.invalid/v1",
		APIKey:     "sk-test",
		Provider:   "openai",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("AddPoolMember: %v", err)
	}
	if err := st.RefreshAccountModels(accountID, []string{"gpt-test"}); err != nil {
		t.Fatalf("RefreshAccountModels: %v", err)
	}
	if err := st.MarkAccountServingFailure(accountID, "EOF", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("MarkAccountServingFailure: %v", err)
	}

	rt := New(store.NewRoutingCache(st))
	resolved, err := rt.Resolve("gpt-test", &poolID, nil)
	if err != nil {
		t.Fatalf("expected expired cooldown to route, got %v", err)
	}
	if resolved.AccountID != accountID {
		t.Fatalf("expected account %d, got %d", accountID, resolved.AccountID)
	}
}
