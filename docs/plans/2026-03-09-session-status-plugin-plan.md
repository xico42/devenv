# Session Status Plugin Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `mark-running` and `notify send` with a unified `devenv plugin handle-claude` command that manages session status via Claude Code hooks, adds desktop notifications via beeep, and distributes as a Claude Code plugin.

**Architecture:** A single `handle-claude` command reads hook JSON from stdin and dispatches based on `hook_event_name`. Core status logic lives in `internal/session/` (agent-agnostic). Desktop notifications use beeep behind an interface in `internal/notify/`. Claude-specific plugin config lives in `plugins/claude/`. Marketplace config at repo root.

**Tech Stack:** Go, Cobra, beeep (desktop notifications), tmux user-defined options, Claude Code plugin system

---

### Task 1: Add beeep dependency

**Step 1: Add the dependency**

Run: `go get github.com/gen2brain/beeep`

**Step 2: Verify it resolved**

Run: `go mod tidy && grep beeep go.mod`
Expected: `github.com/gen2brain/beeep v0.x.x`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add beeep dependency for desktop notifications"
```

---

### Task 2: Update semconv constants

**Files:**
- Modify: `internal/semconv/semconv.go:11-16`
- Modify: `internal/semconv/semconv_test.go` (if exists)

**Step 1: Update the constants**

In `internal/semconv/semconv.go`, rename `TmuxOptionQuestion` to `TmuxOptionAnnotation` and add the status prefix marker:

```go
const (
	SessionEnvVar = "DEVENV_SESSION"

	TmuxOptionStatus    = "@devenv_status"
	TmuxOptionAnnotation = "@devenv_annotation"
	TmuxOptionStartedAt = "@devenv_started_at"

	StatusRunning = "running"
	StatusWaiting = "waiting"

	StatusPrefix = "⚡ "
)
```

**Step 2: Run tests**

Run: `go build ./...`
Expected: Compilation errors in files still referencing `TmuxOptionQuestion` — this is expected; we fix them in subsequent tasks.

**Step 3: Commit**

```bash
git add internal/semconv/semconv.go
git commit -m "refactor: rename TmuxOptionQuestion to TmuxOptionAnnotation, add StatusPrefix"
```

---

### Task 3: Add RenameSession to tmux client

**Files:**
- Modify: `internal/tmux/client.go` (add method after line 100)
- Create: `internal/tmux/client_test.go` (or add to existing)

**Step 1: Write the failing test**

In `internal/tmux/client_test.go`:

```go
func TestRenameSession(t *testing.T) {
	r := &mockRunner{}
	c := tmux.NewClient(r)

	err := c.RenameSession("old-name", "new-name")
	if err != nil {
		t.Fatalf("RenameSession() error = %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	want := []string{"rename-session", "-t", "old-name", "new-name"}
	for i, arg := range want {
		if r.calls[0][i] != arg {
			t.Errorf("arg[%d] = %q, want %q", i, r.calls[0][i], arg)
		}
	}
}

func TestRenameSession_Error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "no such session"}
	c := tmux.NewClient(r)

	err := c.RenameSession("old", "new")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

Note: If `internal/tmux/client_test.go` doesn't exist yet, you'll need to create it with a `mockRunner` struct similar to the one in `internal/session/session_test.go`. Check first.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/... -run TestRenameSession -v`
Expected: FAIL — `RenameSession` not defined

**Step 3: Write the implementation**

Add to `internal/tmux/client.go` after the `SetOption` method (after line 100):

```go
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux/... -run TestRenameSession -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmux/client.go internal/tmux/client_test.go
git commit -m "feat: add RenameSession to tmux client"
```

---

### Task 4: Create notify package

**Files:**
- Create: `internal/notify/notify.go`
- Create: `internal/notify/notify_test.go`

**Step 1: Write the failing test**

Create `internal/notify/notify_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/notify/... -v`
Expected: FAIL — package doesn't exist

**Step 3: Write the implementation**

Create `internal/notify/notify.go`:

```go
package notify

import "github.com/gen2brain/beeep"

// Notifier abstracts desktop notification sending for testability.
type Notifier interface {
	Notify(title, message, appIcon string) error
}

// beeepNotifier wraps beeep.Notify.
type beeepNotifier struct{}

func (b beeepNotifier) Notify(title, message, appIcon string) error {
	return beeep.Notify(title, message, appIcon)
}

// Service sends desktop notifications.
type Service struct {
	notifier Notifier
}

// NewService creates a Service with the given notifier.
func NewService(n Notifier) *Service {
	return &Service{notifier: n}
}

// NewDefaultService creates a Service using beeep for real notifications.
func NewDefaultService() *Service {
	return &Service{notifier: beeepNotifier{}}
}

// Send dispatches a desktop notification.
func (s *Service) Send(title, message string) error {
	return s.notifier.Notify(title, message, "")
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/notify/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/notify/notify.go internal/notify/notify_test.go
git commit -m "feat: add notify package with beeep integration"
```

---

### Task 5: Add SetStatus to session service

**Files:**
- Modify: `internal/session/session.go:163-172` (replace `MarkRunning`)
- Modify: `internal/session/session_test.go` (replace `TestMarkRunning_*` tests)

**Step 1: Write the failing tests**

Replace the `TestMarkRunning_*` tests in `internal/session/session_test.go` (lines 94-142 and 366-375) with these tests:

```go
func TestSetStatus_Running(t *testing.T) {
	// SetStatus("running") sets status, clears annotation, removes prefix
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0},                   // set-option status
		{exitCode: 0},                   // set-option annotation
		{exitCode: 0, stdout: "⚡ myapp-feature\n"}, // list-sessions (for rename check)
		{exitCode: 0},                   // rename-session
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.SetStatus("⚡ myapp-feature", "running", ""); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
}

func TestSetStatus_Waiting(t *testing.T) {
	// SetStatus("waiting") sets status, sets annotation, adds prefix
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0}, // set-option status
		{exitCode: 0}, // set-option annotation
		{exitCode: 0}, // rename-session
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.SetStatus("myapp-feature", "waiting", "Claude needs input"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
}

func TestSetStatus_EmptyName(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc := newService(t, r)

	if err := svc.SetStatus("", "running", ""); err != nil {
		t.Fatalf("SetStatus() on empty name error = %v", err)
	}
	// No tmux calls should be made
	if len(r.calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(r.calls))
	}
}

func TestSetStatus_SuppressesError(t *testing.T) {
	r := &mockRunner{exitCode: 1, err: errors.New("tmux failed")}
	svc := newService(t, r)

	if err := svc.SetStatus("any-session", "running", ""); err != nil {
		t.Fatalf("SetStatus() should suppress errors: %v", err)
	}
}

func TestSetStatus_InvalidStatus(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc := newService(t, r)

	if err := svc.SetStatus("myapp-feature", "invalid", ""); err != nil {
		t.Fatalf("SetStatus() should suppress errors: %v", err)
	}
	// No tmux calls should be made for invalid status
	if len(r.calls) != 0 {
		t.Errorf("expected 0 calls for invalid status, got %d", len(r.calls))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/... -run TestSetStatus -v`
Expected: FAIL — `SetStatus` not defined

**Step 3: Replace MarkRunning with SetStatus**

In `internal/session/session.go`, replace lines 163-172 with:

```go
// SetStatus transitions a session's status and updates the annotation.
// It also renames the tmux session to add/remove the status prefix marker.
// Errors are suppressed — this method always returns nil.
func (s *Service) SetStatus(name, status, annotation string) error {
	if name == "" {
		return nil
	}
	if status != semconv.StatusRunning && status != semconv.StatusWaiting {
		return nil
	}

	_ = s.tmux.SetOption(name, semconv.TmuxOptionStatus, status)
	_ = s.tmux.SetOption(name, semconv.TmuxOptionAnnotation, annotation)

	// Add or remove the status prefix from the tmux session name.
	hasPrefix := strings.HasPrefix(name, semconv.StatusPrefix)
	if status == semconv.StatusRunning && hasPrefix {
		newName := strings.TrimPrefix(name, semconv.StatusPrefix)
		_ = s.tmux.RenameSession(name, newName)
	} else if status != semconv.StatusRunning && !hasPrefix {
		_ = s.tmux.RenameSession(name, semconv.StatusPrefix+name)
	}

	return nil
}
```

Add `"strings"` to the imports in `session.go`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/... -v`
Expected: PASS (the new tests pass; existing tests that reference `MarkRunning` will fail — we fix those next)

**Step 5: Remove old MarkRunning tests**

Delete these test functions from `internal/session/session_test.go`:
- `TestMarkRunning_OK` (lines 94-118)
- `TestMarkRunning_SuppressesError` (lines 120-133)
- `TestMarkRunning_EmptyName` (lines 135-142)
- `TestMarkRunning_SetOptionError` (lines 366-375)

**Step 6: Update List and Show tests**

In `internal/session/session_test.go`, update `TestList_WithOptions` (line 187) to use `question` → the field is still `Question` in `SessionInfo` for now. We'll rename the struct field in Task 7.

Actually — the `SessionInfo.Question` field references `TmuxOptionQuestion` which no longer exists. Update `internal/session/session.go` lines 116 and 138:

```go
// In List():
info.Question, _ = s.tmux.GetOption(name, semconv.TmuxOptionAnnotation)

// In Show():
info.Question, _ = s.tmux.GetOption(name, semconv.TmuxOptionAnnotation)
```

**Step 7: Run all session tests**

Run: `go test ./internal/session/... -v`
Expected: PASS

**Step 8: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: replace MarkRunning with SetStatus, support tmux rename"
```

---

### Task 6: Rename Question to Annotation across codebase

**Files:**
- Modify: `internal/session/session.go:96-103` — rename `Question` field to `Annotation` in `SessionInfo`
- Modify: `internal/session/session.go:116,138` — already updated in Task 5
- Modify: `internal/session/session_test.go` — update `Question` references
- Modify: `cmd/session.go:150-152` — update `show` command output
- Modify: `internal/tui/items.go:26,57,88` — rename `Question` to `Annotation` in `Item` and `agentInfo`
- Modify: `internal/tui/model.go:329` — update `TmuxOptionQuestion` to `TmuxOptionAnnotation`

**Step 1: Rename SessionInfo.Question to Annotation**

In `internal/session/session.go` line 100, change:

```go
// Before:
Question  string

// After:
Annotation string
```

In `internal/session/session.go` lines 116 and 138, change `info.Question` to `info.Annotation`:

```go
info.Annotation, _ = s.tmux.GetOption(name, semconv.TmuxOptionAnnotation)
```

**Step 2: Update session_test.go**

In `TestList_WithOptions` (line 225), change:

```go
// Before:
if sessions[1].Question != "Proceed?" {
    t.Errorf("sessions[1].Question = %q, want Proceed?", sessions[1].Question)
}

// After:
if sessions[1].Annotation != "Proceed?" {
    t.Errorf("sessions[1].Annotation = %q, want Proceed?", sessions[1].Annotation)
}
```

**Step 3: Update cmd/session.go show command**

In `cmd/session.go` lines 150-152, change:

```go
// Before:
if info.Question != "" {
    fmt.Fprintf(w, "Question:\t%s\n", info.Question)
}

// After:
if info.Annotation != "" {
    fmt.Fprintf(w, "Annotation:\t%s\n", info.Annotation)
}
```

**Step 4: Update TUI items**

In `internal/tui/items.go` line 26, change `Question` to `Annotation`:

```go
Annotation string
```

In `internal/tui/items.go` line 57, change `question` to `annotation` in `agentInfo`:

```go
type agentInfo struct {
	status     string
	annotation string
}
```

In `internal/tui/items.go` line 88, update the field assignment:

```go
item.Annotation = agent.annotation
```

**Step 5: Update TUI model**

In `internal/tui/model.go` line 329, change:

```go
// Before:
question, _ := tmuxClient.GetOption(name, semconv.TmuxOptionQuestion)
data.agentSessions[name] = agentInfo{
    status:   status,
    question: question,
}

// After:
annotation, _ := tmuxClient.GetOption(name, semconv.TmuxOptionAnnotation)
data.agentSessions[name] = agentInfo{
    status:     status,
    annotation: annotation,
}
```

**Step 6: Run all tests**

Run: `go test ./... 2>&1 | head -50`
Expected: Everything compiles; tests pass (except possibly config tests referencing notify — we handle those in Task 8)

**Step 7: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go cmd/session.go internal/tui/items.go internal/tui/model.go
git commit -m "refactor: rename Question to Annotation across codebase"
```

---

### Task 7: Update TUI delegate to show annotation and sort waiting first

**Files:**
- Modify: `internal/tui/delegate.go:32,36-93`
- Modify: `internal/tui/items.go:107-115` (sort function)

**Step 1: Update delegate Height and Render**

In `internal/tui/delegate.go`, change `Height()` to return 3 if the item has an annotation, but since Height must be constant for all items in the list delegate, keep it at 2 and render the annotation inline on line 2 after the tags. Actually — the simplest approach is to append a truncated annotation to line 2:

In `delegate.go`, after the tags loop (around line 83), add annotation rendering:

```go
// After the tags are built (line 83), before line2 is finalized:
if item.HasAgent && item.Annotation != "" {
    ann := item.Annotation
    if len(ann) > 60 {
        ann = ann[:57] + "..."
    }
    tags = append(tags, dimStyle.Render(ann))
}
```

**Step 2: Update sort to put waiting sessions first**

In `internal/tui/items.go`, update the sort function (lines 107-115) to sort waiting sessions above running ones within the agent group:

```go
sort.Slice(items, func(i, j int) bool {
    if items[i].Group != items[j].Group {
        return items[i].Group < items[j].Group
    }
    // Within agent group, waiting sorts before running
    if items[i].Group == groupAgent {
        iWaiting := items[i].AgentStatus == semconv.StatusWaiting
        jWaiting := items[j].AgentStatus == semconv.StatusWaiting
        if iWaiting != jWaiting {
            return iWaiting
        }
    }
    if items[i].Project != items[j].Project {
        return items[i].Project < items[j].Project
    }
    return items[i].Branch < items[j].Branch
})
```

**Step 3: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/tui/delegate.go internal/tui/items.go
git commit -m "feat: show annotation in TUI, sort waiting sessions first"
```

---

### Task 8: Delete notify code and mark-running command

**Files:**
- Delete: `cmd/notify.go`
- Delete: `internal/config/notify.go`
- Delete: `internal/config/notify_test.go`
- Delete: `docs/prds/prd-phase-03-cmd-notify.md`
- Modify: `cmd/session.go:232-245,278,285` — remove mark-running command
- Modify: `cmd/root_test.go:36` — remove "notify" from subcommands list
- Modify: `cmd/session_test.go:80-94` — remove mark-running tests
- Modify: `cmd/config.go:78-81,144-148` — remove notify display/get
- Modify: `cmd/config_test.go` — remove notify test cases
- Modify: `internal/config/config.go:27,345-346` — remove Notify field and IsValidKeyPath case

**Step 1: Delete files**

```bash
rm cmd/notify.go
rm internal/config/notify.go
rm internal/config/notify_test.go
rm docs/prds/prd-phase-03-cmd-notify.md
```

**Step 2: Remove Notify from Config struct**

In `internal/config/config.go` line 27, delete:

```go
Notify   NotifyConfig             `toml:"notify"`
```

In `internal/config/config.go` lines 345-346, delete the `case "notify"` block:

```go
case "notify":
    return len(parts) >= 2
```

**Step 3: Remove notify from cmd/config.go**

In `cmd/config.go` lines 78-81, delete:

```go
if cfg.Notify.Provider != "" {
    fmt.Fprintln(w, "\n[notify]")
    fmt.Fprintf(w, "  provider\t= %q\n", cfg.Notify.Provider)
}
```

In `cmd/config.go` lines 144-148, delete the `case "notify"` block:

```go
case "notify":
    if parts[1] == "provider" {
        return c.Notify.Provider, false, nil
    }
    return "", false, fmt.Errorf("use 'config show' for nested notify values")
```

**Step 4: Remove mark-running from cmd/session.go**

Delete lines 232-245 (the `sessionMarkRunningCmd` variable and command definition).
Delete line 234 (`var markRunningSession string`).
Delete line 278 (`sessionMarkRunningCmd.Flags().StringVar(...)`) from `init()`.
Delete line 285 (`sessionCmd.AddCommand(sessionMarkRunningCmd)`) from `init()`.

**Step 5: Update cmd/root_test.go**

In `cmd/root_test.go` line 36, remove `"notify"` from the subcommands list:

```go
// Before:
subcommands := []string{"up", "down", "status", "ssh", "config", "notify", "project", "worktree", "session"}

// After:
subcommands := []string{"up", "down", "status", "ssh", "config", "project", "worktree", "session"}
```

**Step 6: Remove mark-running tests from cmd/session_test.go**

Delete `TestSessionMarkRunning_noSession` (lines 80-86) and `TestSessionMarkRunning_withSession` (lines 88-94).

**Step 7: Remove notify test cases from cmd/config_test.go**

Remove the notify provider line from the config show test fixture (around line 224-225: `[notify]\nprovider = "telegram"`).
Remove the assertion checking for `"telegram"` in show output (around line 246-248).
Remove `TestGetConfigValue_NotifyProvider` function entirely (lines 287-303).
Remove the nested notify key test (lines 305-316).
Remove the `{"notify.provider", true}` and `{"notify.telegram.bot_token", true}` entries from the `IsValidKeyPath` test table (lines 380-381).

**Step 8: Run all tests**

Run: `go test ./... 2>&1 | head -80`
Expected: PASS — everything compiles and passes

**Step 9: Commit**

```bash
git add -A
git commit -m "chore: remove notify command, config, and mark-running"
```

---

### Task 9: Create the plugin handle-claude command

**Files:**
- Create: `cmd/plugin.go`
- Create: `cmd/plugin_test.go`

**Step 1: Write the failing tests**

Create `cmd/plugin_test.go`:

```go
package cmd_test

import (
	"os"
	"strings"
	"testing"
)

func TestPluginHandleClaude_PreToolUse(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	// Pipe JSON to stdin
	r, w, _ := os.Pipe()
	w.WriteString(`{"hook_event_name": "PreToolUse", "session_id": "abc"}`)
	w.Close()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	t.Setenv("DEVENV_SESSION", "myapp-feature")

	// Should succeed silently (fail-open)
	err := runCmd(t, "--config", cfgPath, "plugin", "handle-claude")
	if err != nil {
		t.Fatalf("handle-claude PreToolUse error = %v", err)
	}
}

func TestPluginHandleClaude_Notification(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())

	r, w, _ := os.Pipe()
	w.WriteString(`{"hook_event_name": "Notification", "message": "Claude needs permission", "notification_type": "permission_prompt"}`)
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
	w.WriteString(`{"hook_event_name": "Stop", "last_assistant_message": "I have completed the refactoring. Here is a summary of the changes made across all files."}`)
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
	w.WriteString(`{"hook_event_name": "PreToolUse"}`)
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
	w.WriteString(`{invalid json`)
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
	w.WriteString(`{"hook_event_name": "SessionStart"}`)
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
	w.WriteString(`{"hook_event_name": "Stop", "last_assistant_message": "` + longMessage + `"}`)
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/... -run TestPluginHandleClaude -v`
Expected: FAIL — `plugin` command not registered

**Step 3: Write the implementation**

Create `cmd/plugin.go`:

```go
package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/notify"
	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/tmux"
)

const maxAnnotationLen = 120

// hookInput represents the JSON payload from a Claude Code hook.
type hookInput struct {
	HookEventName        string `json:"hook_event_name"`
	Message              string `json:"message"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

var pluginCmd = &cobra.Command{
	Use:    "plugin",
	Short:  "Plugin commands",
	Hidden: true,
}

var pluginHandleClaudeCmd = &cobra.Command{
	Use:    "handle-claude",
	Short:  "Handle Claude Code hook events",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := os.Getenv(semconv.SessionEnvVar)
		if sessionName == "" {
			return nil
		}

		var input hookInput
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			return nil // fail-open
		}

		tc := tmux.NewClient(tmux.NewRealRunner())
		sesSvc := session.NewService(tc)
		notifySvc := notify.NewDefaultService()

		switch input.HookEventName {
		case "PreToolUse":
			_ = sesSvc.SetStatus(sessionName, semconv.StatusRunning, "")

		case "Notification":
			annotation := truncate(input.Message, maxAnnotationLen)
			_ = sesSvc.SetStatus(sessionName, semconv.StatusWaiting, annotation)
			_ = notifySvc.Send("devenv", annotation)

		case "Stop":
			annotation := truncate(input.LastAssistantMessage, maxAnnotationLen)
			_ = sesSvc.SetStatus(sessionName, semconv.StatusWaiting, annotation)
			_ = notifySvc.Send("devenv", annotation)
		}

		return nil
	},
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func init() {
	pluginCmd.AddCommand(pluginHandleClaudeCmd)
	rootCmd.AddCommand(pluginCmd)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/... -run TestPluginHandleClaude -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... 2>&1 | head -50`
Expected: PASS

**Step 6: Commit**

```bash
git add cmd/plugin.go cmd/plugin_test.go
git commit -m "feat: add plugin handle-claude command"
```

---

### Task 10: Create Claude plugin files and marketplace config

**Files:**
- Create: `.claude-plugin/marketplace.json`
- Create: `plugins/claude/.claude-plugin/plugin.json`
- Create: `plugins/claude/hooks/hooks.json`
- Create: `plugins/claude/README.md`

**Step 1: Create marketplace config**

Create `.claude-plugin/marketplace.json`:

```json
{
  "name": "devenv-plugins",
  "owner": {
    "name": "xico42"
  },
  "plugins": [
    {
      "name": "devenv-session-status",
      "source": "./plugins/claude",
      "description": "Tracks devenv session status and sends desktop notifications when agents need attention"
    }
  ]
}
```

**Step 2: Create plugin manifest**

Create `plugins/claude/.claude-plugin/plugin.json`:

```json
{
  "name": "devenv-session-status",
  "description": "Tracks devenv session status and sends desktop notifications when agents need attention",
  "version": "0.1.0",
  "author": {
    "name": "xico42"
  },
  "repository": "https://github.com/xico42/devenv",
  "license": "MIT"
}
```

**Step 3: Create hooks config**

Create `plugins/claude/hooks/hooks.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "devenv plugin handle-claude"
          }
        ]
      }
    ],
    "Notification": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "devenv plugin handle-claude"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "devenv plugin handle-claude"
          }
        ]
      }
    ]
  }
}
```

**Step 4: Create README**

Create `plugins/claude/README.md`:

```markdown
# devenv-session-status

A Claude Code plugin that tracks devenv session status and sends desktop notifications when agents need attention.

## What it does

- Monitors Claude Code hook events (PreToolUse, Notification, Stop)
- Marks sessions as "waiting" when the agent needs user input
- Marks sessions as "running" when the agent resumes work
- Adds a ⚡ prefix to tmux session names for quick visibility in `Ctrl+b s`
- Sends desktop notifications when a session needs attention
- Shows status annotations in the devenv TUI dashboard

## Prerequisites

- `devenv` must be installed and available in PATH
- Sessions must be started via `devenv session start`

## Installation

### From marketplace

Add the devenv marketplace and install:

```
/plugin marketplace add xico42/devenv
/plugin install devenv-session-status@devenv-plugins
```

### Local development

```bash
claude --plugin-dir ./plugins/claude
```
```

**Step 5: Commit**

```bash
git add .claude-plugin/ plugins/
git commit -m "feat: add Claude Code plugin with marketplace config"
```

---

### Task 11: Final verification

**Step 1: Run full test suite**

Run: `make test`
Expected: PASS

**Step 2: Run lint**

Run: `make lint`
Expected: PASS (fix any lint issues)

**Step 3: Check coverage**

Run: `make coverage`
Expected: >= 80%

**Step 4: Build**

Run: `make build`
Expected: `./devenv` binary built successfully

**Step 5: Smoke test the command**

Run: `echo '{"hook_event_name": "Notification", "message": "test"}' | DEVENV_SESSION=test ./devenv plugin handle-claude`
Expected: Exits 0 (may fail on tmux calls if no session exists, but should not crash)

**Step 6: Commit any fixes**

If any fixes were needed, commit them.

---

## Task Summary

| Task | Description | Dependencies |
|------|-------------|--------------|
| 1 | Add beeep dependency | None |
| 2 | Update semconv constants | None |
| 3 | Add RenameSession to tmux client | None |
| 4 | Create notify package | Task 1 |
| 5 | Add SetStatus to session service | Tasks 2, 3 |
| 6 | Rename Question to Annotation | Tasks 2, 5 |
| 7 | Update TUI delegate and sorting | Task 6 |
| 8 | Delete notify code and mark-running | Tasks 5, 6 |
| 9 | Create plugin handle-claude command | Tasks 4, 5, 8 |
| 10 | Create plugin files and marketplace | Task 9 |
| 11 | Final verification | All |

**Parallelizable:** Tasks 1, 2, 3 can run in parallel. Task 4 depends only on Task 1.
