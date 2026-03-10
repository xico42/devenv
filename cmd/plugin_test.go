package cmd_test

import (
	"os"
	"strings"
	"testing"
)

func TestPluginHandleClaude_UserPromptSubmit(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	// Pipe JSON to stdin
	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "UserPromptSubmit", "prompt": "do something"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	// Should succeed silently (fail-open)
	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude UserPromptSubmit error = %v", err)
	}
}

func TestPluginHandleClaude_Notification(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "Notification", "message": "Claude needs permission", "notification_type": "permission_prompt"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude Notification error = %v", err)
	}
}

func TestPluginHandleClaude_Stop(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "Stop", "last_assistant_message": "I have completed the refactoring. Here is a summary of the changes made across all files."}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude Stop error = %v", err)
	}
}

func TestPluginHandleClaude_NoSession(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "UserPromptSubmit"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	// No DEVENV_SESSION set — should succeed silently
	t.Setenv("DEVENV_SESSION", "")

	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude without session should succeed: %v", err)
	}
}

func TestPluginHandleClaude_MalformedJSON(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{invalid json`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	// Should succeed silently (fail-open)
	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude with bad JSON should succeed: %v", err)
	}
}

func TestPluginHandleClaude_UnknownEvent(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "SessionStart"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude with unknown event should succeed: %v", err)
	}
}

func TestPluginHandleClaude_StopTruncatesAnnotation(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	longMessage := strings.Repeat("a", 200)
	r, w, _ := os.Pipe()
	_, _ = w.WriteString(`{"hook_event_name": "Stop", "last_assistant_message": "` + longMessage + `"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude Stop with long message should succeed: %v", err)
	}
}
