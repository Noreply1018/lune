package store

import "testing"

func TestGetOverviewCountsDegradedPoolsAsHealthy(t *testing.T) {
	st := newTestStore(t)

	poolID, err := st.CreatePool("OpenAI", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	accountID, err := st.CreateAccount(&Account{
		Label:      "degraded-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
		Status:     "degraded",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	if _, err := st.AddPoolMember(poolID, accountID); err != nil {
		t.Fatalf("add pool member: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PoolsTotal != 1 {
		t.Fatalf("expected 1 pool total, got %d", overview.PoolsTotal)
	}
	if overview.PoolsHealthy != 1 {
		t.Fatalf("expected degraded pool to count as healthy, got %d", overview.PoolsHealthy)
	}
}

func TestGetOverviewCountsPoolHealthyWhenAnyRoutableAccountExists(t *testing.T) {
	st := newTestStore(t)

	poolID, err := st.CreatePool("Mixed", 0, true)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	routableID, err := st.CreateAccount(&Account{
		Label:      "healthy-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-healthy",
		Enabled:    true,
		Status:     "healthy",
	})
	if err != nil {
		t.Fatalf("create healthy account: %v", err)
	}

	errorID, err := st.CreateAccount(&Account{
		Label:      "error-account",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-error",
		Enabled:    true,
		Status:     "error",
	})
	if err != nil {
		t.Fatalf("create error account: %v", err)
	}

	if _, err := st.AddPoolMember(poolID, routableID); err != nil {
		t.Fatalf("add healthy member: %v", err)
	}
	if _, err := st.AddPoolMember(poolID, errorID); err != nil {
		t.Fatalf("add error member: %v", err)
	}

	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if overview.PoolsHealthy != 1 {
		t.Fatalf("expected pool with at least one routable account to count as healthy, got %d", overview.PoolsHealthy)
	}
}
