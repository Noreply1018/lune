package store

import (
	"testing"
	"time"
)

func TestUpdateAccountHealthIfUnchangedDoesNotOverwriteNewerHealth(t *testing.T) {
	st := newTestStore(t)

	accountID, err := st.CreateAccount(&Account{
		Label:      "conditional-health-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	observed, err := st.GetAccount(accountID)
	if err != nil || observed == nil {
		t.Fatalf("get observed account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "error", "Rate limit reached"); err != nil {
		t.Fatalf("set newer account health: %v", err)
	}

	updated, err := st.UpdateAccountHealthIfUnchanged(accountID, "healthy", "", *observed)
	if err != nil {
		t.Fatalf("conditional health update: %v", err)
	}
	if updated {
		t.Fatalf("expected conditional update to skip newer health")
	}

	current, err := st.GetAccount(accountID)
	if err != nil || current == nil {
		t.Fatalf("get current account: %v", err)
	}
	if current.Status != "error" || current.LastError != "Rate limit reached" {
		t.Fatalf("newer health was overwritten: status=%q last_error=%q", current.Status, current.LastError)
	}
}

func TestCpaAccountKeyUniquePerService(t *testing.T) {
	st := newTestStore(t)

	serviceID, err := st.CreateCpaService(&CpaService{
		Label:   "CPA",
		BaseURL: "https://cpa.example.com",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	firstID, err := st.CreateAccount(&Account{
		Label:         "first",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create first cpa account: %v", err)
	}
	if firstID == 0 {
		t.Fatalf("expected first account id")
	}
	_, err = st.CreateAccount(&Account{
		Label:         "duplicate",
		SourceKind:    "cpa",
		CpaServiceID:  &serviceID,
		CpaProvider:   "codex",
		CpaAccountKey: "codex-user@example.com-plus",
		Enabled:       true,
	})
	if err == nil {
		t.Fatalf("expected duplicate cpa account key to be rejected")
	}
}

func TestServingFailureDoesNotOverwriteDiscoveryStatus(t *testing.T) {
	st := newTestStore(t)

	accountID, err := st.CreateAccount(&Account{
		Label:      "serving-failure-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.UpdateAccountHealth(accountID, "healthy", ""); err != nil {
		t.Fatalf("set discovery health: %v", err)
	}
	if err := st.MarkAccountServingFailure(accountID, "EOF", time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("mark serving failure: %v", err)
	}
	acc, err := st.GetAccount(accountID)
	if err != nil || acc == nil {
		t.Fatalf("get account: %v", err)
	}
	if acc.Status != "healthy" {
		t.Fatalf("serving failure should not overwrite discovery status, got %q", acc.Status)
	}
	if acc.ServingStatus != "cooldown" {
		t.Fatalf("expected serving cooldown, got %q", acc.ServingStatus)
	}
}
