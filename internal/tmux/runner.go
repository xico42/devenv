package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// Runner executes raw tmux commands. Implement this interface in tests to avoid
// spawning a real tmux process.
type Runner interface {
	Run(args ...string) (stdout, stderr string, exitCode int, err error)
}

// RealRunner executes tmux via os/exec.
type RealRunner struct{}

// NewRealRunner returns a Runner backed by the system tmux binary.
func NewRealRunner() *RealRunner { return &RealRunner{} }

// Run executes tmux with the given arguments. Returns stdout, stderr, exit code,
// and a non-nil err only when the process could not be started at all.
func (r *RealRunner) Run(args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("tmux", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return stdout, stderr, exitErr.ExitCode(), nil
		}
		return stdout, stderr, -1, fmt.Errorf("running tmux %v: %w", args, runErr)
	}
	return stdout, stderr, 0, nil
}
