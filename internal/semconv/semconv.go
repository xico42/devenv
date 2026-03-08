package semconv

import (
	"path/filepath"
	"strings"
)

const (
	SessionEnvVar   = "DEVENV_SESSION"
	DefaultAgentCmd = "claude"
)

func FlattenBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

func SessionName(project, branch string) string {
	return project + "-" + FlattenBranch(branch)
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
