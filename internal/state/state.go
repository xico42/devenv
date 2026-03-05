package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// State holds runtime information about the active droplet.
type State struct {
	DropletID   int       `json:"droplet_id"`
	DropletName string    `json:"droplet_name"`
	TailscaleIP string    `json:"tailscale_ip"`
	PublicIP    string    `json:"public_ip"`
	Region      string    `json:"region"`
	Size        string    `json:"size"`
	Profile     string    `json:"profile"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "devenv", "state.json"), nil
}

func resolvePath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	return defaultPath()
}

// Load reads state from path. If path is empty, uses the XDG default.
// A missing file returns an empty State and nil error.
func Load(path string) (*State, error) {
	p, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state %s: %w", p, err)
	}
	return &s, nil
}

// Save writes state to path, creating directories as needed.
func Save(path string, s *State) error {
	p, err := resolvePath(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}
	return nil
}

// Clear removes the state file. Returns nil if the file doesn't exist.
func Clear(path string) error {
	p, err := resolvePath(path)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing state: %w", err)
	}
	return nil
}
