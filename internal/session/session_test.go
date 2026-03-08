package session_test

import (
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
)

// mockRunner implements tmux.Runner for testing.
type mockRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	calls    [][]string
}

func (m *mockRunner) Run(args ...string) (string, string, int, error) {
	m.calls = append(m.calls, args)
	return m.stdout, m.stderr, m.exitCode, m.err
}

func newService(t *testing.T, r *mockRunner) (*session.Service, string) {
	t.Helper()
	dir := t.TempDir()
	tc := tmux.NewClient(r)
	return session.NewService(tc, dir), dir
}

func TestStart_OK(t *testing.T) {
	dir := t.TempDir()
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 1}, // has-session → not found
		{exitCode: 0}, // new-session → ok
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc, dir)

	wtDir := t.TempDir() // simulate existing worktree
	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    wtDir,
		Cmd:     "claude",
		Env:     map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify state file was written
	s, err := state.LoadSession(dir, "myapp-feature")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if s == nil {
		t.Fatal("state file not written")
	}
	if s.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", s.Status)
	}
	if s.Project != "myapp" {
		t.Errorf("Project = %q, want myapp", s.Project)
	}
}

func TestStart_DuplicateSession(t *testing.T) {
	// has-session returns 0 → session exists
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for duplicate session")
	}
	if !errors.Is(err, session.ErrSessionExists) {
		t.Errorf("error = %v, want ErrSessionExists", err)
	}
}

func TestStart_MissingPath(t *testing.T) {
	// has-session returns 1 → no session
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    "/nonexistent/path",
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !errors.Is(err, session.ErrPathNotFound) {
		t.Errorf("error = %v, want ErrPathNotFound", err)
	}
}

func TestMarkRunning_OK(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	// Write a waiting state
	s := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionWaiting,
		Question:  "Should I proceed?",
		UpdatedAt: time.Now().UTC().Add(-5 * time.Minute),
		StartedAt: time.Now().UTC().Add(-10 * time.Minute),
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkRunning("myapp-feature"); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	got, _ := state.LoadSession(dir, "myapp-feature")
	if got.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Question != "" {
		t.Errorf("Question = %q, want empty", got.Question)
	}
}

func TestMarkRunning_MissingFile(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	// Should not error — silent no-op
	if err := svc.MarkRunning("nonexistent"); err != nil {
		t.Fatalf("MarkRunning() on missing file error = %v", err)
	}
}

func TestMarkRunning_EmptyName(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	if err := svc.MarkRunning(""); err != nil {
		t.Fatalf("MarkRunning() on empty name error = %v", err)
	}
}

// mockResponse is a single canned response for mockRunnerSequence.
type mockResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// mockRunnerSequence returns responses in order, repeating the last one.
type mockRunnerSequence struct {
	responses []mockResponse
	calls     [][]string
	idx       int
}

func (m *mockRunnerSequence) Run(args ...string) (string, string, int, error) {
	m.calls = append(m.calls, args)
	i := m.idx
	if i >= len(m.responses) {
		i = len(m.responses) - 1
	}
	m.idx++
	r := m.responses[i]
	return r.stdout, r.stderr, r.exitCode, r.err
}

func TestList_Empty(t *testing.T) {
	// tmux list-sessions returns exit 1 (no sessions)
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len = %d, want 0", len(sessions))
	}
}

func TestList_WithStateFiles(t *testing.T) {
	// tmux returns two sessions
	r := &mockRunner{exitCode: 0, stdout: "myapp-feature\napi-main\n"}
	svc, dir := newService(t, r)

	// Write state for one of them
	now := time.Now().UTC().Truncate(time.Second)
	ss := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionWaiting,
		Question:  "Proceed?",
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2", len(sessions))
	}

	// Sort by name for deterministic comparison
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	// api-main has no state file → unknown
	if sessions[0].Name != "api-main" {
		t.Errorf("sessions[0].Name = %q, want api-main", sessions[0].Name)
	}
	if sessions[0].Status != "unknown" {
		t.Errorf("sessions[0].Status = %q, want unknown", sessions[0].Status)
	}

	// myapp-feature has state → waiting
	if sessions[1].Status != state.SessionWaiting {
		t.Errorf("sessions[1].Status = %q, want waiting", sessions[1].Status)
	}
	if sessions[1].Question != "Proceed?" {
		t.Errorf("sessions[1].Question = %q, want Proceed?", sessions[1].Question)
	}
}

func TestShow_OK(t *testing.T) {
	// has-session returns 0 → exists
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	now := time.Now().UTC().Truncate(time.Second)
	ss := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	info, err := svc.Show("myapp-feature")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Name != "myapp-feature" {
		t.Errorf("Name = %q, want myapp-feature", info.Name)
	}
	if info.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", info.Status)
	}
}

func TestShow_NotFound(t *testing.T) {
	// has-session returns 1 → not found
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	_, err := svc.Show("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestShow_NoStateFile(t *testing.T) {
	// has-session returns 0 → exists in tmux, but no state file
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	info, err := svc.Show("manual-session")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Status != "unknown" {
		t.Errorf("Status = %q, want unknown", info.Status)
	}
}

func TestStop_OK(t *testing.T) {
	// All tmux calls return 0 (has-session exists, kill-session succeeds)
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	// Write state file
	ss := &state.SessionState{Session: "myapp-feature", Status: state.SessionRunning}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	err := svc.Stop("myapp-feature")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// State file should be deleted
	got, _ := state.LoadSession(dir, "myapp-feature")
	if got != nil {
		t.Error("state file still exists after Stop")
	}
}

func TestStop_NotFound(t *testing.T) {
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	err := svc.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStart_RunnerError(t *testing.T) {
	// has-session returns a runner error (not just non-zero exit)
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc, _ := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestList_RunnerError(t *testing.T) {
	// list-sessions returns a runner error
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc, _ := newService(t, r)

	_, err := svc.List()
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestShow_RunnerError(t *testing.T) {
	// has-session returns a runner error
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc, _ := newService(t, r)

	_, err := svc.Show("myapp-feature")
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestStop_KillError(t *testing.T) {
	// has-session returns 0 (exists), kill-session returns an error
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0}, // has-session → exists
		{exitCode: 1, err: errors.New("kill failed")}, // kill-session → error
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc, t.TempDir())

	err := svc.Stop("myapp-feature")
	if err == nil {
		t.Fatal("expected error when kill fails")
	}
}

func TestMarkRunning_SaveError(t *testing.T) {
	// Use a directory that can't be written to as sessionsDir
	r := &mockRunner{exitCode: 0}
	tc := tmux.NewClient(r)
	// Point sessions dir at a non-existent deeply nested path to simulate save failure
	svc := session.NewService(tc, "/nonexistent/path/sessions")

	// MarkRunning with a name that has no state file → returns nil (silent no-op)
	if err := svc.MarkRunning("any-session"); err != nil {
		t.Fatalf("MarkRunning with no state file should return nil: %v", err)
	}
}

func TestSessionExistsError(t *testing.T) {
	err := &session.SessionExistsError{Name: "myapp-feature"}
	if err.Error() != "session already exists: myapp-feature" {
		t.Errorf("Error() = %q, want %q", err.Error(), "session already exists: myapp-feature")
	}
	if !errors.Is(err, session.ErrSessionExists) {
		t.Error("errors.Is(err, ErrSessionExists) = false, want true")
	}
}

func TestStart_StatError(t *testing.T) {
	// has-session returns 1 (no existing session)
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	// A path with a null byte causes os.Stat to fail with EINVAL, which is not ErrNotExist
	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    "/tmp/\x00invalid",
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
	if errors.Is(err, session.ErrPathNotFound) {
		t.Error("got ErrPathNotFound, expected a different error for invalid path")
	}
}
