package cpa

import "testing"

func TestCancelSessionAllowsAuthorizedStatus(t *testing.T) {
	store := &SessionStore{
		sessions: map[string]*LoginSession{
			"s1": {
				ID:     "s1",
				Status: "authorized",
			},
		},
	}

	if err := store.CancelSession("s1"); err != nil {
		t.Fatalf("cancel session: %v", err)
	}

	if got := store.sessions["s1"].Status; got != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", got)
	}
}
