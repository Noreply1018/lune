package debugcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lune/internal/store"
)

func newDebugTestCommand(t *testing.T) (command, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	authDir := filepath.Join(dir, "cpa-auth")
	if err := os.MkdirAll(authDir, 0755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	st, err := store.New(filepath.Join(dir, "lune.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var out bytes.Buffer
	return command{
		dataDir:    dir,
		dbPath:     filepath.Join(dir, "lune.db"),
		cpaAuthDir: authDir,
		out:        &out,
		errOut:     &bytes.Buffer{},
	}, st
}

func TestDebugCommandsRedactCPAAccountMaterial(t *testing.T) {
	cmd, st := newDebugTestCommand(t)
	svcID, err := st.CreateCpaService(&store.CpaService{
		Label:   "CPA",
		BaseURL: "http://127.0.0.1:8317",
		APIKey:  "sk-secret-service",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	accountKey := "codex-secret@example.com-plus"
	accountID, err := st.CreateAccount(&store.Account{
		Label:         "Sensitive CPA",
		SourceKind:    "cpa",
		CpaServiceID:  &svcID,
		CpaProvider:   "codex",
		CpaAccountKey: accountKey,
		CpaEmail:      "secret@example.com",
		CpaPlanType:   "plus",
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmd.cpaAuthDir, accountKey+".json"), []byte(`{"refresh_token":"refresh-secret"}`), 0600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	var out bytes.Buffer
	cmd.out = &out
	if err := cmd.run([]string{"account", "1"}); err != nil {
		t.Fatalf("account command: %v", err)
	}
	accountOutput := out.String()
	if !strings.Contains(accountOutput, `"cpa_account_key_hash"`) {
		t.Fatalf("expected account key hash, got %s", accountOutput)
	}
	for _, secret := range []string{accountKey, "secret@example.com", "refresh-secret"} {
		if strings.Contains(accountOutput, secret) {
			t.Fatalf("account output leaked %q: %s", secret, accountOutput)
		}
	}
	if !strings.Contains(accountOutput, `"id": `+fmt.Sprint(accountID)) {
		t.Fatalf("expected account id in output, got %s", accountOutput)
	}

	out.Reset()
	if err := cmd.run([]string{"cpa-auth"}); err != nil {
		t.Fatalf("cpa-auth command: %v", err)
	}
	cpaOutput := out.String()
	if !strings.Contains(cpaOutput, `"account_key_hash"`) {
		t.Fatalf("expected auth hash, got %s", cpaOutput)
	}
	for _, secret := range []string{accountKey, "secret@example.com", "refresh-secret"} {
		if strings.Contains(cpaOutput, secret) {
			t.Fatalf("cpa-auth output leaked %q: %s", secret, cpaOutput)
		}
	}
}

func TestDebugOperationDetailIsRedactedAndRecoverable(t *testing.T) {
	cmd, st := newDebugTestCommand(t)
	err := st.RecordOperation(&store.Operation{
		OperationID:   "op-test",
		OperationType: "cpa_import_batch",
		Source:        "test",
		TargetType:    "pool",
		TargetID:      "1",
		Status:        "partial",
		Items: []store.OperationItem{{
			ItemIndex:        1,
			ClientFileName:   "auth.json",
			AccountKeyHash:   "sha256:abcdef123456",
			Status:           "failed",
			RuntimeSync:      "failed",
			ErrorCode:        "runtime_reload_failed",
			SafeErrorMessage: "refresh_token should be redacted",
			Stage:            "request_runtime_reload",
		}},
	})
	if err != nil {
		t.Fatalf("record operation: %v", err)
	}

	var out bytes.Buffer
	cmd.out = &out
	if err := cmd.run([]string{"operation", "op-test"}); err != nil {
		t.Fatalf("operation command: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, `"operation_id": "op-test"`) || !strings.Contains(output, `"stage": "request_runtime_reload"`) {
		t.Fatalf("expected operation detail, got %s", output)
	}
	if strings.Contains(output, "refresh_token should be redacted") {
		t.Fatalf("operation output leaked sensitive-looking message: %s", output)
	}
	if !strings.Contains(output, "REDACTED_SECRET_FIELD") {
		t.Fatalf("expected redacted message marker, got %s", output)
	}
}

func TestDebugOpenReadOnlyDBDoesNotCreateMissingDatabase(t *testing.T) {
	cmd := command{dbPath: filepath.Join(t.TempDir(), "missing", "lune.db")}
	db, err := cmd.openReadOnlyDB()
	if err == nil {
		db.Close()
		t.Fatalf("expected error for missing database")
	}
	if _, statErr := os.Stat(cmd.dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("read-only open created database or unexpected stat error: %v", statErr)
	}
}

func TestDebugOpenReadOnlyDBRejectsWrites(t *testing.T) {
	cmd, st := newDebugTestCommand(t)
	_ = st.Close()
	db, err := cmd.openReadOnlyDB()
	if err != nil {
		t.Fatalf("open read-only db: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE debug_write_probe (id INTEGER)`)
	if err == nil {
		t.Fatalf("expected write to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "readonly") && !strings.Contains(strings.ToLower(err.Error()), "query only") {
		t.Fatalf("expected readonly/query-only error, got %v", err)
	}
}
