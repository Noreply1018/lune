package store

import (
	"path/filepath"
	"testing"
)

func TestListPoolMembersScansFullAccountColumns(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pool-members.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	poolID, _, err := st.CreatePoolWithDefaultToken("Primary", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	serviceID, err := st.CreateCpaService(&CpaService{
		Label:         "CPA",
		BaseURL:       "http://127.0.0.1:8317",
		APIKey:        "sk-cpa",
		ManagementKey: "mgmt",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create cpa service: %v", err)
	}
	accountID, err := st.CreateAccount(&Account{
		Label:                    "Codex",
		SourceKind:               "cpa",
		Provider:                 "codex",
		CpaServiceID:             &serviceID,
		CpaProvider:              "codex",
		CpaAccountKey:            "acct-key",
		CpaEmail:                 "codex@example.com",
		CpaPlanType:              "plus",
		CpaOpenaiID:              "acct-openai",
		CpaCredentialStatus:      "needs_login",
		CpaCredentialReason:      "auth_failed",
		CpaCredentialLastError:   "refresh failed",
		CpaCredentialCheckedAt:   "2026-04-29T00:00:00Z",
		CpaSubscriptionExpiresAt: "2026-05-29T00:00:00Z",
		CpaSubscriptionFetchedAt: "2026-04-29T00:00:00Z",
		CpaSubscriptionLastError: "",
		Enabled:                  true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}

	members, err := st.ListPoolMembers(poolID)
	if err != nil {
		t.Fatalf("list pool members: %v", err)
	}
	if len(members) != 1 || members[0].Account == nil {
		t.Fatalf("expected one member with account, got %+v", members)
	}
	account := members[0].Account
	if account.CpaCredentialStatus != "needs_login" ||
		account.CpaCredentialReason != "auth_failed" ||
		account.CpaSubscriptionExpiresAt != "2026-05-29T00:00:00Z" {
		t.Fatalf("account scan lost CPA fields: %+v", account)
	}
}
