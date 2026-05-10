package store

import "testing"

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
