package tmux

import (
	"fmt"
	"sort"
	"strings"
)

// SessionRecord holds the structured data returned by ListSessions.
type SessionRecord struct {
	Name          string // current tmux session name (may have status prefix)
	CanonicalName string // @codeherd_canonical_name — original name, never changes
	SessionType   string // @codeherd_session_type — "agent" or "shell"
	Status        string // @codeherd_status
	Annotation    string // @codeherd_annotation
	StartedAt     string // @codeherd_started_at (raw RFC3339 string)
}

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

// NewSessionWithEnv creates a detached tmux session with environment variables
// and an initial command.
func (c *Client) NewSessionWithEnv(name, dir string, env map[string]string, cmd string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	// Sort keys for deterministic arg order (testability).
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}
	args = append(args, cmd)
	_, stderr, code, err := c.runner.Run(args...)
	if err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux new-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// GetOption reads a tmux user-defined option from a session.
// Returns empty string (no error) when the option is not set or the session does not exist (tmux exits 1).
func (c *Client) GetOption(session, option string) (string, error) {
	stdout, _, code, err := c.runner.Run("show-option", "-t", session, "-v", option)
	if err != nil {
		return "", fmt.Errorf("tmux show-option: %w", err)
	}
	if code != 0 {
		return "", nil // option not set
	}
	return strings.TrimSpace(stdout), nil
}

// SetOption sets a tmux user-defined option on a session.
func (c *Client) SetOption(session, option, value string) error {
	_, stderr, code, err := c.runner.Run("set-option", "-t", session, option, value)
	if err != nil {
		return fmt.Errorf("tmux set-option: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux set-option: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// RenameSession renames a tmux session.
func (c *Client) RenameSession(oldName, newName string) error {
	_, stderr, code, err := c.runner.Run("rename-session", "-t", oldName, newName)
	if err != nil {
		return fmt.Errorf("tmux rename-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux rename-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// ListSessions returns a SessionRecord for every active tmux session.
// Returns nil (no error) when no sessions exist (tmux exits 1 in that case).
func (c *Client) ListSessions() ([]SessionRecord, error) {
	format := "#{session_name}\t#{@codeherd_canonical_name}\t#{@codeherd_session_type}\t#{@codeherd_status}\t#{@codeherd_annotation}\t#{@codeherd_started_at}"
	stdout, stderr, code, err := c.runner.Run("list-sessions", "-F", format)
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	if code == 1 {
		return nil, nil // no sessions — not an error
	}
	if code != 0 {
		return nil, fmt.Errorf("tmux list-sessions: %s", strings.TrimSpace(stderr))
	}
	var records []SessionRecord
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 6)
		for len(fields) < 6 {
			fields = append(fields, "")
		}
		records = append(records, SessionRecord{
			Name:          fields[0],
			CanonicalName: fields[1],
			SessionType:   fields[2],
			Status:        fields[3],
			Annotation:    fields[4],
			StartedAt:     fields[5],
		})
	}
	return records, nil
}
