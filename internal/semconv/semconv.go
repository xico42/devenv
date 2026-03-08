package semconv

import (
	"path/filepath"
	"strings"
)

const (
	SessionEnvVar = "DEVENV_SESSION"

	TmuxOptionStatus    = "@devenv_status"
	TmuxOptionQuestion  = "@devenv_question"
	TmuxOptionStartedAt = "@devenv_started_at"

	StatusRunning = "running"
	StatusWaiting = "waiting"
)

func FlattenBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

func SessionName(project, branch string) string {
	return project + "-" + FlattenBranch(branch)
}

func ShellSessionName(project, branch string) string {
	return SessionName(project, branch) + "~sh"
}

func CloneDir(projectsDir, repoPath string) string {
	return filepath.Join(projectsDir, repoPath)
}

func WorktreesRoot(projectsDir, repoPath string) string {
	return CloneDir(projectsDir, repoPath) + "__worktrees"
}

func WorktreePath(projectsDir, repoPath, branch string) string {
	return filepath.Join(WorktreesRoot(projectsDir, repoPath), FlattenBranch(branch))
}
