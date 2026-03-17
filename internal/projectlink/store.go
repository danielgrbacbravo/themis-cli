package projectlink

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"themis-cli/internal/state"
	"themis-cli/internal/themis"
)

const (
	projectDirName  = ".themis"
	projectFileName = "project.json"
)

var ErrNotLinked = errors.New("project is not linked")

func ConfigPathFromRepoRoot(repoRoot string) string {
	return filepath.Join(repoRoot, projectDirName, projectFileName)
}

func Save(path string, cfg Config) error {
	canonicalBaseURL, err := themis.NormalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return fmt.Errorf("canonicalize base_url: %w", err)
	}
	canonicalRootURL, err := state.CanonicalizeURL(cfg.LinkedRootURL)
	if err != nil {
		return fmt.Errorf("canonicalize linked_root_url: %w", err)
	}

	cfg.BaseURL = canonicalBaseURL
	cfg.LinkedRootURL = canonicalRootURL
	cfg.SchemaVersion = CurrentSchemaVersion
	if cfg.LinkedRootNodeID == "" {
		cfg.LinkedRootNodeID = state.NodeIDFromCanonicalURL(cfg.LinkedRootURL)
	}
	applyDefaults(&cfg)
	cfg.UpdatedAt = time.Now().UTC()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create project link directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".project-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp project config: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode project config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync project config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp project config: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename project config: %w", err)
	}
	cleanup = false
	return nil
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode project config: %w", err)
	}
	applyDefaults(&cfg)
	return cfg, nil
}

func ResolveByCWD(cwd string) (Config, string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve cwd: %w", err)
	}

	cur := abs
	for {
		path := ConfigPathFromRepoRoot(cur)
		cfg, err := Load(path)
		if err == nil {
			return cfg, path, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return Config{}, "", err
		}

		next := filepath.Dir(cur)
		if next == cur {
			return Config{}, "", ErrNotLinked
		}
		cur = next
	}
}
