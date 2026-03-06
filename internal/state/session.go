package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Session status values.
const (
	SessionRunning = "running"
	SessionWaiting = "waiting"
)

// SessionState holds runtime information about a Claude Code session.
type SessionState struct {
	Session string `json:"session"`
	Project string `json:"project"`
	Branch  string `json:"branch"`
	// Status is the session's current state.
	// Valid values: SessionRunning ("running"), SessionWaiting ("waiting").
	Status    string    `json:"status"`
	Question  string    `json:"question"`
	UpdatedAt time.Time `json:"updated_at"`
	StartedAt time.Time `json:"started_at"`
}

// LoadSession reads a session state file from dir. Returns nil, nil if
// the session does not exist.
func LoadSession(dir, name string) (*SessionState, error) {
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading session %q: %w", name, err)
	}
	var s SessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing session %q: %w", name, err)
	}
	return &s, nil
}

// SaveSession writes a session state file to dir, creating the directory
// if needed. The filename is derived from s.Session.
func SaveSession(dir string, s *SessionState) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}
	path := filepath.Join(dir, s.Session+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing session %q: %w", s.Session, err)
	}
	return nil
}

// ClearSession removes a session state file. Returns nil if the file
// does not exist.
func ClearSession(dir, name string) error {
	path := filepath.Join(dir, name+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing session %q: %w", name, err)
	}
	return nil
}

// ListSessions reads all session state files from dir. Returns an empty
// slice if the directory does not exist or is empty.
func ListSessions(dir string) ([]SessionState, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}
	var sessions []SessionState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		s, err := LoadSession(dir, name)
		if err != nil {
			return nil, err
		}
		if s != nil {
			sessions = append(sessions, *s)
		}
	}
	return sessions, nil
}
