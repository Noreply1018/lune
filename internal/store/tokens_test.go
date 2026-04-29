package store

import "testing"

func TestPoolTokenLifecycleRequiresPoolAndReconcilesOneToken(t *testing.T) {
	st := newTestStore(t)

	if _, err := st.CreateToken(&AccessToken{Name: "global-style", Enabled: true}); err == nil {
		t.Fatalf("expected CreateToken without pool_id to fail")
	}

	poolID, err := st.CreatePool("Primary", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	token, err := st.EnsurePoolToken(poolID)
	if err != nil {
		t.Fatalf("ensure pool token: %v", err)
	}
	if token == nil || token.PoolID == nil || *token.PoolID != poolID {
		t.Fatalf("expected pool-scoped token for pool %d, got %#v", poolID, token)
	}

	again, err := st.EnsurePoolToken(poolID)
	if err != nil {
		t.Fatalf("ensure pool token again: %v", err)
	}
	if again.ID != token.ID {
		t.Fatalf("expected existing token to be reused, got %d then %d", token.ID, again.ID)
	}

	tokens, err := st.ListTokensByPool(poolID)
	if err != nil {
		t.Fatalf("list pool tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected exactly one pool token, got %d", len(tokens))
	}
}
