package state

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	defaultStateDirName  = ".config/themis"
	defaultStateFileName = "state.json"
)

func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, defaultStateDirName, defaultStateFileName), nil
}

func NewEmptyState() State {
	return State{
		SchemaVersion: CurrentSchemaVersion,
		UpdatedAt:     time.Now().UTC(),
		Roots:         []RootRef{},
		Nodes:         map[string]Node{},
	}
}

func Load(path string) (State, error) {
	if err := ensureStateDir(path); err != nil {
		return State{}, err
	}

	lock, err := acquireLock(lockPath(path), false)
	if err != nil {
		return State{}, err
	}
	defer lock.Close()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewEmptyState(), nil
		}
		return State{}, fmt.Errorf("open state file: %w", err)
	}
	defer file.Close()

	var state State
	if err := json.NewDecoder(file).Decode(&state); err != nil {
		return State{}, fmt.Errorf("decode state JSON: %w", err)
	}

	if state.SchemaVersion == 0 {
		state.SchemaVersion = CurrentSchemaVersion
	}
	if state.Roots == nil {
		state.Roots = []RootRef{}
	}
	if state.Nodes == nil {
		state.Nodes = map[string]Node{}
	}

	return state, nil
}

func SaveAtomic(path string, state State, backupOnWrite bool) error {
	if err := ensureStateDir(path); err != nil {
		return err
	}

	lock, err := acquireLock(lockPath(path), true)
	if err != nil {
		return err
	}
	defer lock.Close()

	state.SchemaVersion = CurrentSchemaVersion
	state.UpdatedAt = time.Now().UTC()
	if state.Roots == nil {
		state.Roots = []RootRef{}
	}
	if state.Nodes == nil {
		state.Nodes = map[string]Node{}
	}

	if backupOnWrite {
		if err := backupStateIfExists(path); err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
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
	if err := enc.Encode(state); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode state JSON: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp state file: %w", err)
	}
	cleanup = false

	dirHandle, err := os.Open(dir)
	if err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}

	return nil
}

func backupStateIfExists(path string) error {
	src, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open current state for backup: %w", err)
	}
	defer src.Close()

	backupPath := path + ".bak"
	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup state file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return fmt.Errorf("write backup state file: %w", err)
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("fsync backup state file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close backup state file: %w", err)
	}

	return nil
}

func ensureStateDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	return nil
}

func lockPath(path string) string {
	return path + ".lock"
}

func acquireLock(path string, exclusive bool) (*lockHandle, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), mode); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock state file: %w", err)
	}

	return &lockHandle{File: file}, nil
}

type lockHandle struct {
	*os.File
}

func (l *lockHandle) Close() error {
	if err := syscall.Flock(int(l.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.File.Close()
		return fmt.Errorf("unlock state file: %w", err)
	}
	return l.File.Close()
}
