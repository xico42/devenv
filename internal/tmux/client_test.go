package tmux_test

import (
	"fmt"
	"testing"

	"github.com/xico42/devenv/internal/tmux"
)

type mockRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	lastArgs []string
}

func (m *mockRunner) Run(args ...string) (string, string, int, error) {
	m.lastArgs = args
	return m.stdout, m.stderr, m.exitCode, m.err
}

func TestClient_HasSession_found(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	got, err := c.HasSession("myapp-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
	if r.lastArgs[0] != "has-session" || r.lastArgs[2] != "myapp-feature" {
		t.Errorf("unexpected args: %v", r.lastArgs)
	}
}

func TestClient_HasSession_notFound(t *testing.T) {
	r := &mockRunner{exitCode: 1}
	c := tmux.NewClient(r)
	got, err := c.HasSession("myapp-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for exit code 1")
	}
}

func TestClient_HasSession_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	_, err := c.HasSession("myapp-feature")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_KillSession_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	if err := c.KillSession("myapp-feature"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.lastArgs[0] != "kill-session" || r.lastArgs[2] != "myapp-feature" {
		t.Errorf("unexpected args: %v", r.lastArgs)
	}
}

func TestClient_KillSession_error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "no such session"}
	c := tmux.NewClient(r)
	if err := c.KillSession("myapp-feature"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListSessions_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0, stdout: "foo\nbar\n"}
	c := tmux.NewClient(r)
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 || sessions[0] != "foo" || sessions[1] != "bar" {
		t.Errorf("unexpected sessions: %v", sessions)
	}
}

func TestClient_ListSessions_none(t *testing.T) {
	// tmux exits 1 when no sessions — not an error
	r := &mockRunner{exitCode: 1}
	c := tmux.NewClient(r)
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty, got %v", sessions)
	}
}

func TestClient_NewSession_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	if err := c.NewSession("myapp", "/tmp/myapp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// verify subcommand and key args
	if r.lastArgs[0] != "new-session" {
		t.Errorf("expected new-session, got %v", r.lastArgs)
	}
}

func TestClient_NewSession_error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "duplicate session"}
	c := tmux.NewClient(r)
	if err := c.NewSession("myapp", "/tmp"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_KillSession_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	if err := c.KillSession("myapp-feature"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListSessions_format(t *testing.T) {
	r := &mockRunner{exitCode: 0, stdout: "mysession\n"}
	c := tmux.NewClient(r)
	_, _ = c.ListSessions()
	found := false
	for _, arg := range r.lastArgs {
		if arg == "#{session_name}" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -F #{session_name} in args, got %v", r.lastArgs)
	}
}

func TestClient_NewSession_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	if err := c.NewSession("myapp", "/tmp"); err == nil {
		t.Fatal("expected error on exec failure")
	}
}

func TestClient_ListSessions_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	_, err := c.ListSessions()
	if err == nil {
		t.Fatal("expected error on exec failure")
	}
}

func TestClient_ListSessions_unexpectedCode(t *testing.T) {
	r := &mockRunner{exitCode: 2, stderr: "unexpected error"}
	c := tmux.NewClient(r)
	_, err := c.ListSessions()
	if err == nil {
		t.Fatal("expected error for unexpected exit code")
	}
}

// TestNewRealRunner verifies the constructor returns a non-nil runner.
// This does not execute tmux — it only exercises the constructor.
func TestNewRealRunner(t *testing.T) {
	r := tmux.NewRealRunner()
	if r == nil {
		t.Fatal("NewRealRunner() returned nil")
	}
}

func TestClient_NewSessionWithEnv_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	env := map[string]string{"DEVENV_SESSION": "myapp-feature", "FOO": "bar"}
	err := c.NewSessionWithEnv("myapp-feature", "/tmp/wt", env, "claude --skip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.lastArgs[0] != "new-session" {
		t.Errorf("expected new-session, got %v", r.lastArgs)
	}
	// Verify -d, -s, -c flags are present
	argStr := fmt.Sprintf("%v", r.lastArgs)
	for _, want := range []string{"-d", "-s", "myapp-feature", "-c", "/tmp/wt"} {
		found := false
		for _, a := range r.lastArgs {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in args %s", want, argStr)
		}
	}
}

func TestClient_NewSessionWithEnv_error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "duplicate session"}
	c := tmux.NewClient(r)
	err := c.NewSessionWithEnv("myapp", "/tmp", nil, "claude")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_NewSessionWithEnv_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	err := c.NewSessionWithEnv("myapp", "/tmp", nil, "claude")
	if err == nil {
		t.Fatal("expected error on exec failure")
	}
}

func TestClient_NewSessionWithEnv_envFlags(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	env := map[string]string{"KEY": "val"}
	_ = c.NewSessionWithEnv("s", "/tmp", env, "cmd")
	foundE := false
	for i, a := range r.lastArgs {
		if a == "-e" && i+1 < len(r.lastArgs) && r.lastArgs[i+1] == "KEY=val" {
			foundE = true
			break
		}
	}
	if !foundE {
		t.Errorf("expected -e KEY=val in args, got %v", r.lastArgs)
	}
}

func TestClient_NewSessionWithEnv_cmdIsLastArg(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	_ = c.NewSessionWithEnv("s", "/tmp", nil, "claude --skip")
	last := r.lastArgs[len(r.lastArgs)-1]
	if last != "claude --skip" {
		t.Errorf("last arg = %q, want %q", last, "claude --skip")
	}
}
