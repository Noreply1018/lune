package app

import "testing"

func TestLoadConfigUsesCpaFilesDir(t *testing.T) {
	t.Setenv("LUNE_CPA_FILES_DIR", "/tmp/lune-cpa-files")
	t.Setenv("LUNE_CPA_AUTH_DIR", "/tmp/legacy-cpa-auth")

	cfg := LoadConfig()
	if cfg.CpaAuthDir != "/tmp/lune-cpa-files" {
		t.Fatalf("expected LUNE_CPA_FILES_DIR to win, got %q", cfg.CpaAuthDir)
	}
}

func TestLoadConfigKeepsLegacyCpaAuthDirFallback(t *testing.T) {
	t.Setenv("LUNE_CPA_AUTH_DIR", "/tmp/legacy-cpa-auth")

	cfg := LoadConfig()
	if cfg.CpaAuthDir != "/tmp/legacy-cpa-auth" {
		t.Fatalf("expected legacy LUNE_CPA_AUTH_DIR fallback, got %q", cfg.CpaAuthDir)
	}
}

func TestLoadConfigLetsLegacyCpaAuthDirOverrideImageDefault(t *testing.T) {
	t.Setenv("LUNE_CPA_FILES_DIR", "/app/data/cpa-auth")
	t.Setenv("LUNE_CPA_AUTH_DIR", "/tmp/legacy-cpa-auth")

	cfg := LoadConfig()
	if cfg.CpaAuthDir != "/tmp/legacy-cpa-auth" {
		t.Fatalf("expected legacy LUNE_CPA_AUTH_DIR to override image default, got %q", cfg.CpaAuthDir)
	}
}
