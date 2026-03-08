package session_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
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

func newService(t *testing.T, r *mockRunner) *session.Service {
	t.Helper()
	tc := tmux.NewClient(r)
	return session.NewService(tc)
}

func TestStart_OK(t *testing.T) {
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 1}, // has-session → not found
		{exitCode: 0}, // new-session → ok
		{exitCode: 0}, // set-option status → ok
		{exitCode: 0}, // set-option started_at → ok
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

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
}

func TestStart_DuplicateSession(t *testing.T) {
	// has-session returns 0 → session exists
	r := &mockRunner{exitCode: 0}
	svc := newService(t, r)

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
	svc := newService(t, r)

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
	// MarkRunning calls set-option twice (status + question)
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0}, // set-option status → ok
		{exitCode: 0}, // set-option question → ok
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	if err := svc.MarkRunning("myapp-feature"); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	// Verify correct tmux calls were made
	if len(r2.calls) != 2 {
		t.Fatalf("expected 2 tmux calls, got %d", len(r2.calls))
	}
	// First call: set-option status=running
	if len(r2.calls[0]) < 5 {
		t.Fatalf("unexpected args for first set-option call: %v", r2.calls[0])
	}
	if r2.calls[0][4] != semconv.StatusRunning {
		t.Errorf("set-option status = %q, want %q", r2.calls[0][4], semconv.StatusRunning)
	}
}

func TestMarkRunning_SuppressesError(t *testing.T) {
	// set-option returns an error — MarkRunning must still return nil
	r := &mockRunner{exitCode: 1, err: errors.New("tmux set-option failed")}
	svc := newService(t, r)

	// Should not error — set-option errors are suppressed
	if err := svc.MarkRunning("nonexistent"); err != nil {
		t.Fatalf("MarkRunning() error = %v; want nil (errors suppressed)", err)
	}
	// Verify SetOption was actually attempted
	if len(r.calls) == 0 {
		t.Fatal("expected at least one tmux call, got none")
	}
}

func TestMarkRunning_EmptyName(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc := newService(t, r)

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
	svc := newService(t, r)

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len = %d, want 0", len(sessions))
	}
}

func TestList_WithOptions(t *testing.T) {
	// list-sessions returns two sessions, then for each session 3x get-option calls
	// Order: list-sessions, myapp-feature: status, question, started_at, api-main: status, question, started_at
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: "myapp-feature\napi-main\n"}, // list-sessions
		{exitCode: 0, stdout: semconv.StatusWaiting},       // get-option status (myapp-feature)
		{exitCode: 0, stdout: "Proceed?"},                  // get-option question (myapp-feature)
		{exitCode: 0, stdout: ""},                          // get-option started_at (myapp-feature)
		{exitCode: 0, stdout: ""},                          // get-option status (api-main) → empty
		{exitCode: 0, stdout: ""},                          // get-option question (api-main) → empty
		{exitCode: 0, stdout: ""},                          // get-option started_at (api-main) → empty
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

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

	// api-main has no options set → empty status
	if sessions[0].Name != "api-main" {
		t.Errorf("sessions[0].Name = %q, want api-main", sessions[0].Name)
	}
	if sessions[0].Status != "" {
		t.Errorf("sessions[0].Status = %q, want empty", sessions[0].Status)
	}

	// myapp-feature has options → waiting
	if sessions[1].Status != semconv.StatusWaiting {
		t.Errorf("sessions[1].Status = %q, want waiting", sessions[1].Status)
	}
	if sessions[1].Question != "Proceed?" {
		t.Errorf("sessions[1].Question = %q, want Proceed?", sessions[1].Question)
	}
}

func TestShow_OK(t *testing.T) {
	// has-session → exists, then 3x get-option calls
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0}, // has-session → exists
		{exitCode: 0, stdout: semconv.StatusRunning},  // get-option status
		{exitCode: 0, stdout: ""},                     // get-option question
		{exitCode: 0, stdout: "2024-01-01T00:00:00Z"}, // get-option started_at
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	info, err := svc.Show("myapp-feature")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Name != "myapp-feature" {
		t.Errorf("Name = %q, want myapp-feature", info.Name)
	}
	if info.Status != semconv.StatusRunning {
		t.Errorf("Status = %q, want running", info.Status)
	}
	if info.StartedAt.IsZero() {
		t.Error("StartedAt should be non-zero")
	}
}

func TestShow_NotFound(t *testing.T) {
	// has-session returns 1 → not found
	r := &mockRunner{exitCode: 1}
	svc := newService(t, r)

	_, err := svc.Show("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestShow_NoOptions(t *testing.T) {
	// has-session returns 0 → exists, get-option calls return empty
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0}, // has-session → exists
		{exitCode: 0}, // get-option status → empty
		{exitCode: 0}, // get-option question → empty
		{exitCode: 0}, // get-option started_at → empty
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	info, err := svc.Show("manual-session")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Status != "" {
		t.Errorf("Status = %q, want empty", info.Status)
	}
}

func TestStop_OK(t *testing.T) {
	// All tmux calls return 0 (has-session exists, kill-session succeeds)
	r := &mockRunner{exitCode: 0}
	svc := newService(t, r)

	err := svc.Stop("myapp-feature")
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStop_NotFound(t *testing.T) {
	r := &mockRunner{exitCode: 1}
	svc := newService(t, r)

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
	svc := newService(t, r)

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
	svc := newService(t, r)

	_, err := svc.List()
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestShow_RunnerError(t *testing.T) {
	// has-session returns a runner error
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc := newService(t, r)

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
	svc := session.NewService(tc)

	err := svc.Stop("myapp-feature")
	if err == nil {
		t.Fatal("expected error when kill fails")
	}
}

func TestMarkRunning_SetOptionError(t *testing.T) {
	// set-option calls fail — errors are suppressed, MarkRunning always returns nil
	r := &mockRunner{exitCode: 1, err: errors.New("tmux set-option failed")}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.MarkRunning("any-session"); err != nil {
		t.Fatalf("MarkRunning with set-option error should return nil: %v", err)
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
	svc := newService(t, r)

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
