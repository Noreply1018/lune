package health

import "testing"

func TestParseCodexSubscriptionExpiryFromAccountCheck(t *testing.T) {
	raw := []byte(`{
		"accounts": [
			{
				"account_id": "acct_other",
				"entitlement": {"expires_at": "2031-01-01T00:00:00Z"}
			},
			{
				"account_id": "acct_target",
				"account_plan": {"subscription_expires_at_timestamp": 1893456000}
			}
		]
	}`)

	got, err := parseCodexSubscriptionExpiry(raw, "acct_target")
	if err != nil {
		t.Fatalf("parse expiry: %v", err)
	}
	if got != "2030-01-01T00:00:00Z" {
		t.Fatalf("expected target account expiry, got %q", got)
	}
}

func TestParseCodexSubscriptionExpiryIgnoresUnrelatedExpiresAt(t *testing.T) {
	raw := []byte(`{
		"access_token": {"expires_at": "2030-01-01T00:00:00Z"},
		"profile": {"email": "user@example.com"}
	}`)

	if got, err := parseCodexSubscriptionExpiry(raw, ""); err == nil {
		t.Fatalf("expected no subscription expiry, got %q", got)
	}
}

func TestParseCodexSubscriptionExpiryDoesNotUseOtherAccount(t *testing.T) {
	raw := []byte(`{
		"accounts": [
			{"account_id": "acct_other", "entitlement": {"expires_at": "2031-01-01T00:00:00Z"}},
			{"account_id": "acct_target", "profile": {"name": "target"}}
		]
	}`)

	if got, err := parseCodexSubscriptionExpiry(raw, "acct_target"); err == nil {
		t.Fatalf("expected no target subscription expiry, got %q", got)
	}
}
