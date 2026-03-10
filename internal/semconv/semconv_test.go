package semconv_test

import (
	"strings"
	"testing"

	"github.com/xico42/devenv/internal/semconv"
)

func TestFlattenBranch(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"main", "main"},
		{"feature/login", "feature-login"},
		{"fix/auth/token", "fix-auth-token"},
		{"no-slash", "no-slash"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := semconv.FlattenBranch(tt.input); got != tt.want {
			t.Errorf("FlattenBranch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionName(t *testing.T) {
	tests := []struct {
		project, branch, want string
	}{
		{"myapp", "feature", "myapp-feature"},
		{"myapp", "feature/login", "myapp-feature-login"},
		{"api", "fix/auth/token", "api-fix-auth-token"},
	}
	for _, tt := range tests {
		if got := semconv.SessionName(tt.project, tt.branch); got != tt.want {
			t.Errorf("SessionName(%q, %q) = %q, want %q", tt.project, tt.branch, got, tt.want)
		}
	}
}

func TestCloneDir(t *testing.T) {
	got := semconv.CloneDir("/home/user/projects", "github.com/user/myapp")
	want := "/home/user/projects/github.com/user/myapp"
	if got != want {
		t.Errorf("CloneDir() = %q, want %q", got, want)
	}
}

func TestWorktreesRoot(t *testing.T) {
	got := semconv.WorktreesRoot("/home/user/projects", "github.com/user/myapp")
	want := "/home/user/projects/github.com/user/myapp__worktrees"
	if got != want {
		t.Errorf("WorktreesRoot() = %q, want %q", got, want)
	}
}

func TestWorktreePath(t *testing.T) {
	got := semconv.WorktreePath("/home/user/projects", "github.com/user/myapp", "feature/login")
	want := "/home/user/projects/github.com/user/myapp__worktrees/feature-login"
	if got != want {
		t.Errorf("WorktreePath() = %q, want %q", got, want)
	}
}

func TestShellSessionName(t *testing.T) {
	tests := []struct {
		project, branch, want string
	}{
		{"myapp", "feature", "myapp-feature~sh"},
		{"myapp", "feature/login", "myapp-feature-login~sh"},
		{"api", "fix/auth/token", "api-fix-auth-token~sh"},
	}
	for _, tt := range tests {
		if got := semconv.ShellSessionName(tt.project, tt.branch); got != tt.want {
			t.Errorf("ShellSessionName(%q, %q) = %q, want %q", tt.project, tt.branch, got, tt.want)
		}
	}
}

func TestTmuxOptionConstants(t *testing.T) {
	// Verify constants have @ prefix (required for tmux user options).
	for _, opt := range []string{
		semconv.TmuxOptionStatus,
		semconv.TmuxOptionAnnotation,
		semconv.TmuxOptionStartedAt,
	} {
		if !strings.HasPrefix(opt, "@") {
			t.Errorf("tmux option %q must start with @", opt)
		}
	}
}

func TestConstants(t *testing.T) {
	if semconv.SessionEnvVar != "DEVENV_SESSION" {
		t.Errorf("SessionEnvVar = %q, want DEVENV_SESSION", semconv.SessionEnvVar)
	}
}

func TestNewSemconvConstants(t *testing.T) {
	if semconv.TmuxOptionCanonicalName != "@devenv_canonical_name" {
		t.Errorf("TmuxOptionCanonicalName = %q", semconv.TmuxOptionCanonicalName)
	}
	if semconv.TmuxOptionSessionType != "@devenv_session_type" {
		t.Errorf("TmuxOptionSessionType = %q", semconv.TmuxOptionSessionType)
	}
	if semconv.SessionTypeAgent != "agent" {
		t.Errorf("SessionTypeAgent = %q", semconv.SessionTypeAgent)
	}
	if semconv.SessionTypeShell != "shell" {
		t.Errorf("SessionTypeShell = %q", semconv.SessionTypeShell)
	}
}
