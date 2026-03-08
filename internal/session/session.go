package session

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
)

var (
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
	ErrPathNotFound    = errors.New("worktree path not found")
)

// SessionExistsError is returned by Start when a tmux session with the same
// name already exists. It wraps ErrSessionExists for errors.Is compatibility
// and carries the session Name for structured access by callers.
type SessionExistsError struct {
	Name string
}

func (e *SessionExistsError) Error() string {
	return ErrSessionExists.Error() + ": " + e.Name
}

func (e *SessionExistsError) Unwrap() error {
	return ErrSessionExists
}

// Service manages devenv tmux sessions and their persisted state.
type Service struct {
	tmux        *tmux.Client
	sessionsDir string
}

// NewService creates a Service using the given tmux client and sessions state directory.
func NewService(tmux *tmux.Client, sessionsDir string) *Service {
	return &Service{tmux: tmux, sessionsDir: sessionsDir}
}

// StartRequest holds parameters for starting a new session.
type StartRequest struct {
	Project string
	Branch  string
	Path    string
	Cmd     string
	Env     map[string]string
	Attach  bool
}

// Start creates a new detached tmux session for the given project/branch,
// sets the DEVENV_SESSION env var, and persists a running session state file.
// Returns ErrSessionExists if a session with the derived name already exists.
// Returns ErrPathNotFound if Path does not exist on disk.
func (s *Service) Start(req StartRequest) error {
	name := semconv.SessionName(req.Project, req.Branch)

	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if exists {
		return &SessionExistsError{Name: name}
	}

	if _, err := os.Stat(req.Path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrPathNotFound, req.Path)
		}
		return fmt.Errorf("checking path: %w", err)
	}

	env := make(map[string]string)
	for k, v := range req.Env {
		env[k] = v
	}
	env[semconv.SessionEnvVar] = name

	if err := s.tmux.NewSessionWithEnv(name, req.Path, env, req.Cmd); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	now := time.Now().UTC()
	ss := &state.SessionState{
		Session:   name,
		Project:   req.Project,
		Branch:    req.Branch,
		Status:    state.SessionRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(s.sessionsDir, ss); err != nil {
		return fmt.Errorf("saving session state: %w", err)
	}

	return nil
}

// SessionInfo holds display information about a tmux session.
type SessionInfo struct {
	Name      string
	Project   string
	Branch    string
	Status    string
	Question  string
	StartedAt time.Time
	UpdatedAt time.Time
}

// List returns a SessionInfo for every active tmux session.
// Sessions with no state file get Status "unknown".
func (s *Service) List() ([]SessionInfo, error) {
	names, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing tmux sessions: %w", err)
	}

	var result []SessionInfo
	for _, name := range names {
		info := SessionInfo{Name: name, Status: "unknown"}
		ss, err := state.LoadSession(s.sessionsDir, name)
		if err == nil && ss != nil {
			info.Project = ss.Project
			info.Branch = ss.Branch
			info.Status = ss.Status
			info.Question = ss.Question
			info.StartedAt = ss.StartedAt
			info.UpdatedAt = ss.UpdatedAt
		}
		result = append(result, info)
	}
	return result, nil
}

// Show returns the SessionInfo for a single named tmux session.
// Returns ErrSessionNotFound if the session does not exist in tmux.
// Sessions with no state file get Status "unknown".
func (s *Service) Show(name string) (*SessionInfo, error) {
	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}

	info := &SessionInfo{Name: name, Status: "unknown"}
	ss, err := state.LoadSession(s.sessionsDir, name)
	if err == nil && ss != nil {
		info.Project = ss.Project
		info.Branch = ss.Branch
		info.Status = ss.Status
		info.Question = ss.Question
		info.StartedAt = ss.StartedAt
		info.UpdatedAt = ss.UpdatedAt
	}
	return info, nil
}

// Stop kills the named tmux session and removes its state file.
// Returns ErrSessionNotFound if the session does not exist in tmux.
func (s *Service) Stop(name string) error {
	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}

	if err := s.tmux.KillSession(name); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	if err := state.ClearSession(s.sessionsDir, name); err != nil {
		return fmt.Errorf("clearing session state: %w", err)
	}

	return nil
}

// MarkRunning transitions a session's persisted state to running and clears
// any pending question. If the session state file does not exist, it is a
// silent no-op. Errors are suppressed — this method always returns nil.
func (s *Service) MarkRunning(name string) error {
	if name == "" {
		return nil
	}
	ss, err := state.LoadSession(s.sessionsDir, name)
	if err != nil {
		return nil // silent — never fail
	}
	if ss == nil {
		return nil // no state file — no-op
	}
	ss.Status = state.SessionRunning
	ss.Question = ""
	ss.UpdatedAt = time.Now().UTC()
	if err := state.SaveSession(s.sessionsDir, ss); err != nil {
		return nil // silent — never fail
	}
	return nil
}
