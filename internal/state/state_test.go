package state_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xico42/devenv/internal/state"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Load(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if s == nil {
		t.Fatal("Load() returned nil state")
	}
	if s.DropletID != 0 {
		t.Errorf("DropletID = %d, want 0", s.DropletID)
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := state.Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for corrupt file")
	}
	if !strings.Contains(err.Error(), "parsing state") {
		t.Errorf("error %q should mention 'parsing state'", err.Error())
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := &state.State{
		DropletID:   12345,
		DropletName: "devenv-test",
		TailscaleIP: "100.64.0.1",
		PublicIP:    "1.2.3.4",
		Region:      "nyc3",
		Size:        "s-2vcpu-4gb",
		Profile:     "default",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Status:      "active",
	}
	if err := state.Save(path, s); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.DropletID != s.DropletID {
		t.Errorf("DropletID = %d, want %d", got.DropletID, s.DropletID)
	}
	if got.DropletName != s.DropletName {
		t.Errorf("DropletName = %q, want %q", got.DropletName, s.DropletName)
	}
	if got.TailscaleIP != s.TailscaleIP {
		t.Errorf("TailscaleIP = %q, want %q", got.TailscaleIP, s.TailscaleIP)
	}
	if got.PublicIP != s.PublicIP {
		t.Errorf("PublicIP = %q, want %q", got.PublicIP, s.PublicIP)
	}
	if got.Region != s.Region {
		t.Errorf("Region = %q, want %q", got.Region, s.Region)
	}
	if got.Size != s.Size {
		t.Errorf("Size = %q, want %q", got.Size, s.Size)
	}
	if got.Profile != s.Profile {
		t.Errorf("Profile = %q, want %q", got.Profile, s.Profile)
	}
	if got.Status != s.Status {
		t.Errorf("Status = %q, want %q", got.Status, s.Status)
	}
	if !got.CreatedAt.Equal(s.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, s.CreatedAt)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Clear on missing file must not error
	if err := state.Clear(path); err != nil {
		t.Fatalf("Clear() on missing file error = %v", err)
	}

	if err := state.Save(path, &state.State{DropletID: 1}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := state.Clear(path); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	s, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() after Clear() error = %v", err)
	}
	if s.DropletID != 0 {
		t.Errorf("DropletID after Clear = %d, want 0", s.DropletID)
	}
}
