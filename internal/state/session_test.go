package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xico42/devenv/internal/state"
)

func TestLoadSession_Missing(t *testing.T) {
	dir := t.TempDir()
	s, err := state.LoadSession(dir, "nonexistent")
	if err != nil {
		t.Fatalf("LoadSession() error = %v, want nil", err)
	}
	if s != nil {
		t.Errorf("LoadSession() = %v, want nil for missing session", s)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	s := &state.SessionState{
		Session:   "my-session",
		Project:   "myapp",
		Branch:    "feature/auth",
		Status:    state.SessionRunning,
		UpdatedAt: now,
		StartedAt: now,
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "my-session")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession() returned nil")
	}
	if got.Session != s.Session {
		t.Errorf("Session = %q, want %q", got.Session, s.Session)
	}
	if got.Project != s.Project {
		t.Errorf("Project = %q, want %q", got.Project, s.Project)
	}
	if got.Branch != s.Branch {
		t.Errorf("Branch = %q, want %q", got.Branch, s.Branch)
	}
	if got.Status != state.SessionRunning {
		t.Errorf("Status = %q, want %q", got.Status, state.SessionRunning)
	}
	if !got.StartedAt.Equal(s.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, s.StartedAt)
	}
}

func TestSaveSession_WithQuestion(t *testing.T) {
	dir := t.TempDir()
	s := &state.SessionState{
		Session:   "waiting-session",
		Project:   "myapp",
		Branch:    "main",
		Status:    state.SessionWaiting,
		Question:  "Should I refactor the auth module?",
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "waiting-session")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got.Status != state.SessionWaiting {
		t.Errorf("Status = %q, want %q", got.Status, state.SessionWaiting)
	}
	if got.Question != s.Question {
		t.Errorf("Question = %q, want %q", got.Question, s.Question)
	}
}

func TestSaveSession_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	s := &state.SessionState{
		Session: "test",
		Status:  state.SessionRunning,
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() should create dir, got error = %v", err)
	}
	got, err := state.LoadSession(dir, "test")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession() returned nil after save to nested dir")
	}
}

func TestClearSession(t *testing.T) {
	dir := t.TempDir()

	// Clear on missing session must not error
	if err := state.ClearSession(dir, "nonexistent"); err != nil {
		t.Fatalf("ClearSession() on missing error = %v", err)
	}

	// Save then clear
	s := &state.SessionState{Session: "to-clear", Status: state.SessionRunning}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := state.ClearSession(dir, "to-clear"); err != nil {
		t.Fatalf("ClearSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "to-clear")
	if err != nil {
		t.Fatalf("LoadSession() after clear error = %v", err)
	}
	if got != nil {
		t.Errorf("LoadSession() after clear = %v, want nil", got)
	}
}

func TestListSessions_Empty(t *testing.T) {
	dir := t.TempDir()
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestListSessions_Multiple(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"alpha", "beta"} {
		s := &state.SessionState{
			Session:   name,
			Project:   "proj",
			Status:    state.SessionRunning,
			StartedAt: now,
			UpdatedAt: now,
		}
		if err := state.SaveSession(dir, s); err != nil {
			t.Fatalf("SaveSession(%q) error = %v", name, err)
		}
	}
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	names := map[string]bool{}
	for _, s := range sessions {
		names[s.Session] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("sessions = %v, want alpha and beta", names)
	}
}

func TestListSessions_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() on missing dir error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}

// TestLoadSession_CorruptFile exercises the JSON parse error branch.
func TestLoadSession_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := state.LoadSession(dir, "bad")
	if err == nil {
		t.Fatal("LoadSession() on corrupt file = nil, want error")
	}
}

// TestLoadSession_ReadError exercises the non-ErrNotExist read error path by
// placing a directory where the session file is expected.
func TestLoadSession_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory named "mysession.json" so ReadFile fails.
	if err := os.Mkdir(filepath.Join(dir, "mysession.json"), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	_, err := state.LoadSession(dir, "mysession")
	if err == nil {
		t.Fatal("LoadSession() on directory = nil, want error")
	}
}

// TestListSessions_SkipsNonJSON verifies that non-.json entries in the
// sessions dir are silently ignored (exercises the continue branch).
func TestListSessions_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	// Write a non-.json file and a valid session.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := &state.SessionState{Session: "ok", Status: state.SessionRunning}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("len(sessions) = %d, want 1", len(sessions))
	}
}

// TestListSessions_SkipsSubdirs verifies that subdirectories inside the
// sessions dir are silently ignored (exercises the IsDir() continue branch).
func TestListSessions_SkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with a .json name.
	if err := os.Mkdir(filepath.Join(dir, "subdir.json"), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}

// TestSaveSession_WriteError exercises the WriteFile error branch by using a
// directory path where the session file is expected.
func TestSaveSession_WriteError(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a directory named "mysession.json" so WriteFile fails.
	sessionPath := filepath.Join(dir, "mysession.json")
	if err := os.Mkdir(sessionPath, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	s := &state.SessionState{Session: "mysession", Status: state.SessionRunning}
	err := state.SaveSession(dir, s)
	if err == nil {
		t.Fatal("SaveSession() on directory path = nil, want error")
	}
}

// TestListSessions_ReadDirError exercises the non-ErrNotExist ReadDir error by
// pointing ListSessions at a file instead of a directory.
func TestListSessions_ReadDirError(t *testing.T) {
	// Create a file where a directory is expected.
	f, err := os.CreateTemp("", "notadir")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	_, err = state.ListSessions(f.Name())
	if err == nil {
		t.Fatal("ListSessions() on file path = nil, want error")
	}
}

// TestClearSession_RemoveError exercises the non-ErrNotExist remove error
// by placing a directory where the session file is expected.
func TestClearSession_RemoveError(t *testing.T) {
	dir := t.TempDir()
	// Create a directory named "mysession.json"; os.Remove on a non-empty
	// dir returns an error that is not ErrNotExist.
	sessionDir := filepath.Join(dir, "mysession.json")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Put a file inside so the dir is non-empty and Remove definitely fails.
	if err := os.WriteFile(filepath.Join(sessionDir, "x"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err := state.ClearSession(dir, "mysession")
	if err == nil {
		t.Fatal("ClearSession() on non-empty directory = nil, want error")
	}
}
