package remote_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/xico42/devenv/internal/remote"
)

// mockClient implements remote.Client for testing.
var _ remote.Client = (*mockClient)(nil)

type mockClient struct {
	runFn       func(ctx context.Context, cmd string) (string, string, error)
	runStreamFn func(ctx context.Context, cmd string, w io.Writer) error
	closeFn     func() error
}

func (m *mockClient) Run(ctx context.Context, cmd string) (string, string, error) {
	return m.runFn(ctx, cmd)
}
func (m *mockClient) RunStream(ctx context.Context, cmd string, w io.Writer) error {
	return m.runStreamFn(ctx, cmd, w)
}
func (m *mockClient) Close() error { return m.closeFn() }

func TestMockClient_Run(t *testing.T) {
	tests := []struct {
		name       string
		fn         func(context.Context, string) (string, string, error)
		wantStdout string
		wantErr    bool
	}{
		{
			name:       "success",
			fn:         func(_ context.Context, _ string) (string, string, error) { return "hello\n", "", nil },
			wantStdout: "hello\n",
		},
		{
			name:    "error",
			fn:      func(_ context.Context, _ string) (string, string, error) { return "", "", errors.New("exit 1") },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &mockClient{runFn: tt.fn}
			got, _, err := c.Run(context.Background(), "echo hello")
			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantStdout {
				t.Errorf("Run() stdout = %q, want %q", got, tt.wantStdout)
			}
		})
	}
}

func TestMockClient_RunStream(t *testing.T) {
	c := &mockClient{
		runStreamFn: func(_ context.Context, _ string, w io.Writer) error {
			_, err := io.WriteString(w, "streaming\n")
			return err
		},
	}
	var buf strings.Builder
	if err := c.RunStream(context.Background(), "cmd", &buf); err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if buf.String() != "streaming\n" {
		t.Errorf("output = %q, want %q", buf.String(), "streaming\n")
	}
}

func TestMockClient_Close(t *testing.T) {
	closed := false
	c := &mockClient{closeFn: func() error { closed = true; return nil }}
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !closed {
		t.Error("Close() did not mark closed")
	}
}
