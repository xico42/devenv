package semconv_test

import (
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

func TestConstants(t *testing.T) {
	if semconv.SessionEnvVar != "DEVENV_SESSION" {
		t.Errorf("SessionEnvVar = %q, want DEVENV_SESSION", semconv.SessionEnvVar)
	}
	if semconv.DefaultAgentCmd != "claude" {
		t.Errorf("DefaultAgentCmd = %q, want claude", semconv.DefaultAgentCmd)
	}
}
