package projectlink

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	repoRoot := t.TempDir()
	path := ConfigPathFromRepoRoot(repoRoot)

	in := Config{
		BaseURL:       "https://themis.housing.rug.nl/",
		LinkedRootURL: "https://themis.housing.rug.nl/course/2025-2026/os/",
		Preferences: Preferences{
			DefaultRefreshDepth:          2,
			AutoRefreshOnOpen:            true,
			ShowStaleWarningAfterMinutes: 60,
		},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if out.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("schema mismatch: %d", out.SchemaVersion)
	}
	if out.BaseURL != "https://themis.housing.rug.nl" {
		t.Fatalf("base_url mismatch: %s", out.BaseURL)
	}
	if out.LinkedRootURL != "https://themis.housing.rug.nl/course/2025-2026/os" {
		t.Fatalf("linked_root_url mismatch: %s", out.LinkedRootURL)
	}
	if out.LinkedRootNodeID == "" {
		t.Fatalf("expected linked_root_node_id to be set")
	}
	if out.Preferences.DefaultRefreshDepth != 2 || !out.Preferences.AutoRefreshOnOpen || out.Preferences.ShowStaleWarningAfterMinutes != 60 {
		t.Fatalf("preferences mismatch: %#v", out.Preferences)
	}
	if out.UpdatedAt.IsZero() {
		t.Fatalf("expected updated_at to be set")
	}
}

func TestResolveByCWD_FindsNearestLinkedProject(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	nested := filepath.Join(repoRoot, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	cfg := Config{
		BaseURL:       "https://themis.housing.rug.nl",
		LinkedRootURL: "https://themis.housing.rug.nl/course/2025-2026/os",
	}
	path := ConfigPathFromRepoRoot(repoRoot)
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	resolved, resolvedPath, err := ResolveByCWD(nested)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolvedPath != path {
		t.Fatalf("path mismatch: want=%s got=%s", path, resolvedPath)
	}
	if resolved.LinkedRootURL != cfg.LinkedRootURL {
		t.Fatalf("root mismatch: want=%s got=%s", cfg.LinkedRootURL, resolved.LinkedRootURL)
	}
}

func TestResolveByCWD_NotLinked(t *testing.T) {
	_, _, err := ResolveByCWD(t.TempDir())
	if !errors.Is(err, ErrNotLinked) {
		t.Fatalf("expected ErrNotLinked, got: %v", err)
	}
}

func TestLoad_AppliesDefaultPreferences(t *testing.T) {
	repoRoot := t.TempDir()
	path := ConfigPathFromRepoRoot(repoRoot)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	content := []byte(`{
  "schema_version": 1,
  "base_url": "https://themis.housing.rug.nl",
  "linked_root_url": "https://themis.housing.rug.nl/course/2025-2026/os",
  "preferences": {}
}`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Preferences.DefaultRefreshDepth != 1 {
		t.Fatalf("default refresh depth mismatch: %d", cfg.Preferences.DefaultRefreshDepth)
	}
	if cfg.Preferences.ShowStaleWarningAfterMinutes != 120 {
		t.Fatalf("default stale warning mismatch: %d", cfg.Preferences.ShowStaleWarningAfterMinutes)
	}
}
