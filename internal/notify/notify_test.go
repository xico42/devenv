package notify_test

import (
	"testing"

	"github.com/xico42/devenv/internal/notify"
)

type mockNotifier struct {
	calls []struct{ title, message string }
}

func (m *mockNotifier) Notify(title, message, appIcon string) error {
	m.calls = append(m.calls, struct{ title, message string }{title, message})
	return nil
}

func TestSend(t *testing.T) {
	mock := &mockNotifier{}
	svc := notify.NewService(mock)

	err := svc.Send("devenv", "Claude needs input")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0].title != "devenv" {
		t.Errorf("title = %q, want %q", mock.calls[0].title, "devenv")
	}
	if mock.calls[0].message != "Claude needs input" {
		t.Errorf("message = %q, want %q", mock.calls[0].message, "Claude needs input")
	}
}

func TestSend_EmptyMessage(t *testing.T) {
	mock := &mockNotifier{}
	svc := notify.NewService(mock)

	err := svc.Send("devenv", "")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	// Should still call through — empty message is valid
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
}
