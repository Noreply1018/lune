package store

import (
	"path/filepath"
	"testing"
)

func TestUpdateAccountDiagnosticWithEvidencePreservesSchedulerOverride(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "lune.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	accountID, err := st.CreateAccount(&Account{
		Label:      "override",
		SourceKind: "openai_compat",
		BaseURL:    "https://example.com/v1",
		APIKey:     "sk-test",
		Enabled:    true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if err := st.UpdateAccountDiagnostic(accountID, AccountDiagnosticUpdate{
		StableDiagnosticStatus: "banned",
		LastProbeStatus:        "upstream_banned_signal",
		SchedulerStatus:        "eligible",
		SchedulerOverride:      "manual override",
		SafeSummary:            "operator override",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnostic: %v", err)
	}

	if err := st.UpdateAccountDiagnosticWithEvidence(accountID, AccountDiagnosticUpdate{
		StableDiagnosticStatus: "unknown",
		LastProbeStatus:        "transient_error",
		SafeSummary:            "transient upstream error",
		PreserveStableStatus:   true,
	}, AccountDiagnosticEvidenceInput{
		ProbeType:           "routing_observation",
		Stage:               "model_request",
		NormalizedErrorCode: "upstream_transient_error",
		SafeMessage:         "transient upstream error",
	}); err != nil {
		t.Fatalf("UpdateAccountDiagnosticWithEvidence: %v", err)
	}

	diag, err := st.GetAccountDiagnostic(accountID)
	if err != nil {
		t.Fatalf("GetAccountDiagnostic: %v", err)
	}
	if diag.StableDiagnosticStatus != "banned" || diag.SchedulerStatus != "eligible" || diag.SchedulerOverride != "manual override" {
		t.Fatalf("expected preserve to keep stable scheduler override, got %+v", diag)
	}
	if diag.LastProbeStatus != "transient_error" || len(diag.Evidence) != 1 || diag.Evidence[0].NormalizedErrorCode != "upstream_transient_error" {
		t.Fatalf("expected transient evidence without stable override loss, got %+v", diag)
	}
}
