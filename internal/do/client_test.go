package do_test

import (
	"testing"

	"github.com/xico42/codeherd/internal/do"
)

func TestNew_EmptyToken(t *testing.T) {
	_, err := do.New("")
	if err == nil {
		t.Fatal("New(\"\") error = nil, want error")
	}
}

func TestNew_WithToken(t *testing.T) {
	c, err := do.New("fake-token")
	if err != nil {
		t.Fatalf("New(\"fake-token\") error = %v, want nil", err)
	}
	if c == nil {
		t.Fatal("New() returned nil client")
	}
	if c.Droplets == nil {
		t.Error("Client.Droplets is nil, want non-nil")
	}
}
