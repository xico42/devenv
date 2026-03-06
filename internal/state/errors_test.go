package state

// errors_test.go exercises error branches in Save, SaveSession, and ListSessions
// that require OS-level failures (read-only dirs, bad paths).
// This file is in package state (not state_test) so it can directly test
// package-internal helpers.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSave_MkdirAllError exercises the MkdirAll error branch in Save
// by pointing the state at a path whose parent directory cannot be created.
func TestSave_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at "parent" so MkdirAll("parent/sub") fails.
	parentFile := filepath.Join(dir, "parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	path := filepath.Join(parentFile, "sub", "state.json")
	err := Save(path, &State{DropletID: 1})
	if err == nil {
		t.Fatal("Save() = nil, want error when MkdirAll fails")
	}
}

// TestSave_WriteFileError exercises the WriteFile error branch in Save
// by making the state directory read-only.
func TestSave_WriteFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions; skipping")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "state.json")
	err := Save(path, &State{DropletID: 1})
	if err == nil {
		t.Fatal("Save() = nil, want error when WriteFile fails")
	}
}

// TestSaveSession_MkdirAllError exercises the MkdirAll error branch in
// SaveSession by pointing the sessions dir at a path that can't be created.
func TestSaveSession_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at "sessions" so MkdirAll("sessions/sub") fails.
	badDir := filepath.Join(dir, "sessions")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err := SaveSession(badDir, &SessionState{Session: "test", Status: SessionRunning})
	if err == nil {
		t.Fatal("SaveSession() = nil, want error when MkdirAll fails")
	}
}

// TestSaveSession_WriteFileError exercises the WriteFile error branch in
// SaveSession by making the sessions directory read-only.
func TestSaveSession_WriteFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions; skipping")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := SaveSession(dir, &SessionState{Session: "test", Status: SessionRunning})
	if err == nil {
		t.Fatal("SaveSession() = nil, want error when WriteFile fails")
	}
}

// TestListSessions_ReadDirError exercises the non-ErrNotExist ReadDir error
// branch by pointing the sessions dir at a regular file.
func TestListSessions_ReadDirError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file where a directory is expected.
	file := filepath.Join(dir, "sessions")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := ListSessions(file)
	if err == nil {
		t.Fatal("ListSessions() = nil, want error when ReadDir fails on a file")
	}
}

// TestDefaultPath exercises the defaultPath function directly.
func TestDefaultPath(t *testing.T) {
	p, err := defaultPath()
	if err != nil {
		t.Fatalf("defaultPath() error = %v", err)
	}
	if p == "" {
		t.Error("defaultPath() = empty, want a non-empty path")
	}
}
