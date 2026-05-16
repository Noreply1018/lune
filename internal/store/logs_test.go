package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func newLogsTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "logs-test.db"))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st
}

func TestInsertLogSanitizesAndFoldsRepeatedErrors(t *testing.T) {
	st := newLogsTestStore(t)

	msg := "authorization: Bearer token-123 x-api-key=sk-secret prompt:{\"text\":\"hello\"} " + strings.Repeat("x", 5000)
	for i := 0; i < 6; i++ {
		if err := st.InsertLog(&RequestLog{
			RequestID:       "req",
			AccessTokenName: "token",
			ModelRequested:  "gpt-test",
			ModelActual:     "gpt-test",
			AccountID:       1,
			StatusCode:      500,
			Success:         false,
			ErrorMessage:    msg,
			SourceKind:      "openai_compat",
		}); err != nil {
			t.Fatalf("insert log %d: %v", i, err)
		}
	}

	logs, total, err := st.ListLogs(10, 0)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected folded single log, got total=%d len=%d", total, len(logs))
	}
	got := logs[0]
	if got.ErrorRepeatCount != 6 {
		t.Fatalf("expected repeat count 6, got %d", got.ErrorRepeatCount)
	}
	if len(got.ErrorMessage) > maxRequestLogErrorMessageBytes {
		t.Fatalf("expected sanitized error <= %d bytes, got %d", maxRequestLogErrorMessageBytes, len(got.ErrorMessage))
	}
	if strings.Contains(strings.ToLower(got.ErrorMessage), "bearer ") || strings.Contains(got.ErrorMessage, "sk-secret") {
		t.Fatalf("expected secrets to be redacted, got %q", got.ErrorMessage)
	}
}

func TestUsageExcludesDiagnosticLogsByDefault(t *testing.T) {
	st := newLogsTestStore(t)
	if err := st.InsertLog(&RequestLog{
		RequestID:       "diag",
		AccessTokenName: "token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		AccountID:       1,
		StatusCode:      200,
		Success:         true,
		Diagnostic:      true,
		SourceKind:      "openai_compat",
		InputTokens:     1,
		OutputTokens:    2,
	}); err != nil {
		t.Fatalf("insert diagnostic log: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:       "norm",
		AccessTokenName: "token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		AccountID:       2,
		StatusCode:      200,
		Success:         true,
		SourceKind:      "openai_compat",
		InputTokens:     3,
		OutputTokens:    4,
	}); err != nil {
		t.Fatalf("insert normal log: %v", err)
	}

	stats, err := st.GetUsageSummary(UsageFilter{})
	if err != nil {
		t.Fatalf("usage summary: %v", err)
	}
	if stats.TotalRequests != 1 || stats.TotalInputTokens != 3 || stats.TotalOutputTokens != 4 {
		t.Fatalf("expected diagnostic log to be excluded, got %+v", stats)
	}
	if len(stats.ByAccount) != 1 || stats.ByAccount[0].AccountID != 2 {
		t.Fatalf("expected only normal account in usage summary, got %+v", stats.ByAccount)
	}
}

func TestRequestLogKeepsDeletedAccountLabelSnapshot(t *testing.T) {
	st := newLogsTestStore(t)
	accountID, err := st.CreateAccount(&Account{
		Label:      "Deleted CPA Account",
		SourceKind: "cpa",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.InsertLog(&RequestLog{
		RequestID:       "history",
		AccessTokenName: "token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		AccountID:       accountID,
		StatusCode:      200,
		Success:         true,
		SourceKind:      "cpa",
	}); err != nil {
		t.Fatalf("insert log: %v", err)
	}
	if err := st.DeleteAccount(accountID); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	logs, total, err := st.GetUsage(UsageFilter{IncludeDiagnostic: true})
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("expected one historical log, total=%d logs=%+v", total, logs)
	}
	if logs[0].AccountLabel != "Deleted CPA Account" {
		t.Fatalf("expected deleted account snapshot label, got %q", logs[0].AccountLabel)
	}
}

func TestDiagnosticErrorsDoNotFoldIntoOrdinaryErrors(t *testing.T) {
	st := newLogsTestStore(t)
	base := RequestLog{
		RequestID:       "same-error",
		AccessTokenName: "token",
		ModelRequested:  "gpt-test",
		ModelActual:     "gpt-test",
		AccountID:       1,
		StatusCode:      503,
		Success:         false,
		ErrorMessage:    "same upstream failure",
		SourceKind:      "openai_compat",
	}
	if err := st.InsertLog(&base); err != nil {
		t.Fatalf("insert ordinary log: %v", err)
	}
	diag := base
	diag.RequestID = "same-error-diagnostic"
	diag.Diagnostic = true
	if err := st.InsertLog(&diag); err != nil {
		t.Fatalf("insert diagnostic log: %v", err)
	}

	logs, total, err := st.ListLogs(10, 0)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if total != 2 || len(logs) != 2 {
		t.Fatalf("expected ordinary and diagnostic errors to stay separate, total=%d logs=%+v", total, logs)
	}
	for _, log := range logs {
		if log.ErrorRepeatCount != 1 {
			t.Fatalf("expected no cross-folding, got %+v", log)
		}
	}
}

func TestOverviewExcludesDiagnosticLogs(t *testing.T) {
	st := newLogsTestStore(t)
	if err := st.InsertLog(&RequestLog{
		RequestID:      "diagnostic-overview",
		ModelRequested: "gpt-test",
		ModelActual:    "gpt-test",
		StatusCode:     200,
		Success:        true,
		Diagnostic:     true,
		SourceKind:     "openai_compat",
		LatencyMs:      10,
	}); err != nil {
		t.Fatalf("insert diagnostic log: %v", err)
	}
	overview, err := st.GetOverview()
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.RequestsToday != 0 || overview.SuccessRateToday != 0 || overview.AvgLatencyToday != 0 {
		t.Fatalf("overview should exclude diagnostic logs, got %+v", overview)
	}
}
