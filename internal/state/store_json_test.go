package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_MissingFileReturnsEmptyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json")

	state, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if state.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("unexpected schema version: %d", state.SchemaVersion)
	}
	if len(state.Roots) != 0 {
		t.Fatalf("expected empty roots, got %d", len(state.Roots))
	}
	if len(state.Nodes) != 0 {
		t.Fatalf("expected empty nodes, got %d", len(state.Nodes))
	}
}

func TestSaveAtomic_ThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "state.json")

	in := NewEmptyState()
	in.BaseURL = "https://themis.housing.rug.nl"
	in.CatalogRootURL = "https://themis.housing.rug.nl/course"

	if err := SaveAtomic(path, in, false); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if out.BaseURL != in.BaseURL {
		t.Fatalf("base_url mismatch: want=%q got=%q", in.BaseURL, out.BaseURL)
	}
	if out.CatalogRootURL != in.CatalogRootURL {
		t.Fatalf("catalog_root_url mismatch: want=%q got=%q", in.CatalogRootURL, out.CatalogRootURL)
	}
}

func TestSaveAtomic_BackupOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "state.json")

	first := NewEmptyState()
	first.BaseURL = "https://themis.housing.rug.nl"
	if err := SaveAtomic(path, first, false); err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	second := NewEmptyState()
	second.BaseURL = "https://themis.housing.rug.nl/v2"
	if err := SaveAtomic(path, second, true); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup file, stat failed: %v", err)
	}

	backup, err := Load(backupPath)
	if err != nil {
		t.Fatalf("load backup failed: %v", err)
	}
	if backup.BaseURL != first.BaseURL {
		t.Fatalf("backup mismatch: want=%q got=%q", first.BaseURL, backup.BaseURL)
	}
}
