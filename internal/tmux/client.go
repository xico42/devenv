package tmux

import (
	"fmt"
	"strings"
)

// Client provides typed tmux operations built on a Runner.
type Client struct {
	runner Runner
}

// NewClient creates a Client using the given Runner.
func NewClient(r Runner) *Client {
	return &Client{runner: r}
}

// HasSession reports whether a tmux session with the given name exists.
// tmux exits with code 1 to signal "no session" — this is not an error.
func (c *Client) HasSession(name string) (bool, error) {
	_, _, code, err := c.runner.Run("has-session", "-t", name)
	if err != nil {
		return false, fmt.Errorf("tmux has-session: %w", err)
	}
	return code == 0, nil
}

// KillSession terminates the named tmux session.
func (c *Client) KillSession(name string) error {
	_, stderr, code, err := c.runner.Run("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux kill-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// NewSession creates a detached tmux session with the given name and start directory.
func (c *Client) NewSession(name, dir string) error {
	_, stderr, code, err := c.runner.Run("new-session", "-d", "-s", name, "-c", dir)
	if err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux new-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// ListSessions returns the names of all active tmux sessions.
// Returns nil (no error) when no sessions exist (tmux exits 1 in that case).
func (c *Client) ListSessions() ([]string, error) {
	stdout, stderr, code, err := c.runner.Run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	if code == 1 {
		return nil, nil // no sessions — not an error
	}
	if code != 0 {
		return nil, fmt.Errorf("tmux list-sessions: %s", strings.TrimSpace(stderr))
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}
