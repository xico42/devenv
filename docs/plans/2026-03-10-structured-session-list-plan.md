# Structured Session List Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace name-based session heuristics with a single structured `tmux list-sessions` call that embeds canonical name, session type, and status as tmux options, making all session lookups stable across renames.

**Architecture:** Store `@devenv_canonical_name` and `@devenv_session_type` on every devenv tmux session at creation. Replace `ListSessions() []string` with `ListSessions() []SessionRecord`, fetching all six fields in one format-string call. All callers — `SetStatus`, `List`, `Show`, `Stop`, and TUI refresh — derive the actual tmux session name from the canonical name stored in the record, eliminating `HasSession` probes and per-session `GetOption` calls.

**Tech Stack:** Go, tmux user-defined options, bubbles/bubbletea v2, cobra.

---

### Task 1: semconv constants

**Files:**
- Modify: `internal/semconv/semconv.go`
- Test: `internal/semconv/semconv_test.go`

**Step 1: Write the failing test**

Add to `internal/semconv/semconv_test.go`:

```go
func TestNewSemconvConstants(t *testing.T) {
	if semconv.TmuxOptionCanonicalName != "@devenv_canonical_name" {
		t.Errorf("TmuxOptionCanonicalName = %q", semconv.TmuxOptionCanonicalName)
	}
	if semconv.TmuxOptionSessionType != "@devenv_session_type" {
		t.Errorf("TmuxOptionSessionType = %q", semconv.TmuxOptionSessionType)
	}
	if semconv.SessionTypeAgent != "agent" {
		t.Errorf("SessionTypeAgent = %q", semconv.SessionTypeAgent)
	}
	if semconv.SessionTypeShell != "shell" {
		t.Errorf("SessionTypeShell = %q", semconv.SessionTypeShell)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/semconv/...
```
Expected: FAIL — undefined constants.

**Step 3: Add constants to semconv.go**

In the `const` block in `internal/semconv/semconv.go`, add after the existing option constants:

```go
TmuxOptionCanonicalName = "@devenv_canonical_name"
TmuxOptionSessionType   = "@devenv_session_type"

SessionTypeAgent = "agent"
SessionTypeShell = "shell"
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/semconv/...
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/semconv/semconv.go internal/semconv/semconv_test.go
git commit -m "feat: add canonical name and session type semconv constants"
```

---

### Task 2: tmux.SessionRecord and updated ListSessions

**Files:**
- Modify: `internal/tmux/client.go`
- Modify: `internal/tmux/client_test.go`

**Step 1: Write failing tests**

Replace the existing `TestClient_ListSessions_ok` and `TestClient_ListSessions_format` tests in `internal/tmux/client_test.go`. Also add a test for multi-field parsing:

```go
func TestClient_ListSessions_ok(t *testing.T) {
	line := "myapp-feat\tmyapp-feat\tagent\trunning\tdoing stuff\t2026-01-01T00:00:00Z"
	r := &mockRunner{exitCode: 0, stdout: line + "\n"}
	c := tmux.NewClient(r)
	records, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.Name != "myapp-feat" {
		t.Errorf("Name = %q, want myapp-feat", rec.Name)
	}
	if rec.CanonicalName != "myapp-feat" {
		t.Errorf("CanonicalName = %q, want myapp-feat", rec.CanonicalName)
	}
	if rec.SessionType != "agent" {
		t.Errorf("SessionType = %q, want agent", rec.SessionType)
	}
	if rec.Status != "running" {
		t.Errorf("Status = %q, want running", rec.Status)
	}
	if rec.Annotation != "doing stuff" {
		t.Errorf("Annotation = %q, want doing stuff", rec.Annotation)
	}
	if rec.StartedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("StartedAt = %q, want 2026-01-01T00:00:00Z", rec.StartedAt)
	}
}

func TestClient_ListSessions_prefixedAndShell(t *testing.T) {
	lines := "⚡ myapp-feat\tmyapp-feat\tagent\twaiting\tneed input\t\n" +
		"myapp-feat~sh\tmyapp-feat\tshell\t\t\t\n"
	r := &mockRunner{exitCode: 0, stdout: lines}
	c := tmux.NewClient(r)
	records, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len = %d, want 2", len(records))
	}
	if records[0].Name != "⚡ myapp-feat" {
		t.Errorf("records[0].Name = %q", records[0].Name)
	}
	if records[0].CanonicalName != "myapp-feat" {
		t.Errorf("records[0].CanonicalName = %q", records[0].CanonicalName)
	}
	if records[1].SessionType != "shell" {
		t.Errorf("records[1].SessionType = %q, want shell", records[1].SessionType)
	}
}

func TestClient_ListSessions_nonDevenv(t *testing.T) {
	// Non-devenv sessions have empty option fields.
	r := &mockRunner{exitCode: 0, stdout: "other-session\t\t\t\t\t\n"}
	c := tmux.NewClient(r)
	records, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len = %d, want 1", len(records))
	}
	if records[0].CanonicalName != "" {
		t.Errorf("CanonicalName = %q, want empty", records[0].CanonicalName)
	}
	if records[0].SessionType != "" {
		t.Errorf("SessionType = %q, want empty", records[0].SessionType)
	}
}

func TestClient_ListSessions_format(t *testing.T) {
	r := &mockRunner{exitCode: 0, stdout: "s\t\t\t\t\t\n"}
	c := tmux.NewClient(r)
	_, _ = c.ListSessions()
	argStr := fmt.Sprintf("%v", r.lastArgs)
	for _, want := range []string{"#{session_name}", "#{@devenv_canonical_name}", "#{@devenv_session_type}"} {
		if !strings.Contains(argStr, want) {
			t.Errorf("expected %q in args %s", want, argStr)
		}
	}
}
```

Add `"strings"` to the import block in `client_test.go`.

**Step 2: Run test to verify it fails**

```bash
go test ./internal/tmux/...
```
Expected: FAIL — `ListSessions` returns `[]string`, not `[]SessionRecord`.

**Step 3: Implement SessionRecord and new ListSessions**

In `internal/tmux/client.go`, add the struct before the `Client` type definition:

```go
// SessionRecord holds the structured data returned by ListSessions.
type SessionRecord struct {
	Name          string // current tmux session name (may have status prefix)
	CanonicalName string // @devenv_canonical_name — original name, never changes
	SessionType   string // @devenv_session_type — "agent" or "shell"
	Status        string // @devenv_status
	Annotation    string // @devenv_annotation
	StartedAt     string // @devenv_started_at (raw RFC3339 string)
}
```

Replace the `ListSessions` method:

```go
// ListSessions returns a SessionRecord for every active tmux session.
// Returns nil (no error) when no sessions exist (tmux exits 1 in that case).
func (c *Client) ListSessions() ([]SessionRecord, error) {
	format := "#{session_name}\t#{@devenv_canonical_name}\t#{@devenv_session_type}\t#{@devenv_status}\t#{@devenv_annotation}\t#{@devenv_started_at}"
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
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/tmux/...
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tmux/client.go internal/tmux/client_test.go
git commit -m "feat: replace ListSessions string slice with SessionRecord struct"
```

---

### Task 3: session.Start — list-based duplicate check and new options

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

**Step 1: Update tests**

In `internal/session/session_test.go`, replace `TestStart_OK`, `TestStart_DuplicateSession`, `TestStart_MissingPath`, `TestStart_RunnerError`, and `TestStart_StatError`:

```go
func TestStart_OK(t *testing.T) {
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 1}, // list-sessions → no sessions (exit 1 = empty)
		{exitCode: 0}, // new-session → ok
		{exitCode: 0}, // set-option status
		{exitCode: 0}, // set-option started_at
		{exitCode: 0}, // set-option canonical_name
		{exitCode: 0}, // set-option session_type
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	wtDir := t.TempDir()
	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    wtDir,
		Cmd:     "claude",
		Env:     map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(r2.calls) != 6 {
		t.Errorf("expected 6 tmux calls, got %d: %v", len(r2.calls), r2.calls)
	}
}

func TestStart_DuplicateSession(t *testing.T) {
	// list-sessions returns a record with the same canonical name
	line := "myapp-feature\tmyapp-feature\tagent\trunning\t\t\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line},
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for duplicate session")
	}
	if !errors.Is(err, session.ErrSessionExists) {
		t.Errorf("error = %v, want ErrSessionExists", err)
	}
}

func TestStart_DuplicateSession_Prefixed(t *testing.T) {
	// list-sessions returns a prefixed (waiting) session with the same canonical name
	line := "⚡ myapp-feature\tmyapp-feature\tagent\twaiting\t\t\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line},
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if !errors.Is(err, session.ErrSessionExists) {
		t.Errorf("error = %v, want ErrSessionExists", err)
	}
}

func TestStart_MissingPath(t *testing.T) {
	// list-sessions exits 1 (no sessions) — not an error
	r := &mockRunner{exitCode: 1}
	svc := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    "/nonexistent/path",
		Cmd:     "claude",
	})
	if !errors.Is(err, session.ErrPathNotFound) {
		t.Errorf("error = %v, want ErrPathNotFound", err)
	}
}

func TestStart_RunnerError(t *testing.T) {
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestStart_StatError(t *testing.T) {
	// list-sessions exits 1 (no sessions)
	r := &mockRunner{exitCode: 1}
	svc := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    "/tmp/\x00invalid",
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
	if errors.Is(err, session.ErrPathNotFound) {
		t.Error("got ErrPathNotFound, expected a different error for invalid path")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/...
```
Expected: FAIL — `Start` still uses `HasSession` and does not set the two new options.

**Step 3: Update session.Start**

In `internal/session/session.go`, replace the duplicate-check block at the top of `Start` and add the two new `SetOption` calls after session creation:

```go
func (s *Service) Start(req StartRequest) error {
	name := semconv.SessionName(req.Project, req.Branch)

	// Check for existing session by canonical name (handles prefixed names too).
	records, err := s.tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	for _, r := range records {
		if r.CanonicalName == name {
			return &SessionExistsError{Name: name}
		}
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
	_ = s.tmux.SetOption(name, semconv.TmuxOptionStatus, semconv.StatusRunning)
	_ = s.tmux.SetOption(name, semconv.TmuxOptionStartedAt, now.Format(time.RFC3339))
	_ = s.tmux.SetOption(name, semconv.TmuxOptionCanonicalName, name)
	_ = s.tmux.SetOption(name, semconv.TmuxOptionSessionType, semconv.SessionTypeAgent)

	return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/session/...
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: store canonical name and session type on new agent sessions"
```

---

### Task 4: session.List — use records

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

**Step 1: Update tests**

Replace `TestList_Empty`, `TestList_WithOptions`, and `TestList_RunnerError` in `internal/session/session_test.go`:

```go
func TestList_Empty(t *testing.T) {
	r := &mockRunner{exitCode: 1} // list-sessions exit 1 = no sessions
	svc := newService(t, r)

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len = %d, want 0", len(sessions))
	}
}

func TestList_WithOptions(t *testing.T) {
	lines := "⚡ myapp-feature\tmyapp-feature\tagent\twaiting\tProceed?\t\n" +
		"api-main\tapi-main\tagent\t\t\t\n" +
		"api-main~sh\tapi-main\tshell\t\t\t\n" // shell session — should be excluded
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: lines},
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2 (agent sessions only, shell excluded)", len(sessions))
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	if sessions[0].Name != "api-main" {
		t.Errorf("sessions[0].Name = %q, want api-main", sessions[0].Name)
	}
	if sessions[0].Status != "" {
		t.Errorf("sessions[0].Status = %q, want empty", sessions[0].Status)
	}
	if sessions[1].Name != "myapp-feature" {
		t.Errorf("sessions[1].Name = %q, want myapp-feature (canonical, no prefix)", sessions[1].Name)
	}
	if sessions[1].Status != semconv.StatusWaiting {
		t.Errorf("sessions[1].Status = %q, want waiting", sessions[1].Status)
	}
	if sessions[1].Annotation != "Proceed?" {
		t.Errorf("sessions[1].Annotation = %q, want Proceed?", sessions[1].Annotation)
	}
}

func TestList_RunnerError(t *testing.T) {
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc := newService(t, r)

	_, err := svc.List()
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/...
```
Expected: FAIL — `List` still calls `GetOption` per session.

**Step 3: Update session.List**

Replace the `List` method in `internal/session/session.go`:

```go
// List returns a SessionInfo for every active agent tmux session.
func (s *Service) List() ([]SessionInfo, error) {
	records, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing tmux sessions: %w", err)
	}

	var result []SessionInfo
	for _, r := range records {
		if r.SessionType != semconv.SessionTypeAgent {
			continue
		}
		info := SessionInfo{
			Name:       r.CanonicalName,
			Status:     r.Status,
			Annotation: r.Annotation,
		}
		if r.StartedAt != "" {
			info.StartedAt, _ = time.Parse(time.RFC3339, r.StartedAt)
		}
		result = append(result, info)
	}
	return result, nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/session/...
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "refactor: session.List uses structured list-sessions record"
```

---

### Task 5: session.Show and session.Stop — list-based lookup

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

`Show` will set `TmuxName` on `SessionInfo` so that callers that need the actual tmux session name (e.g., attach) can use it. `Stop` will find the actual session name and kill it correctly even when it has the status prefix.

**Step 1: Add TmuxName to SessionInfo**

In `internal/session/session.go`, add `TmuxName` to the `SessionInfo` struct:

```go
type SessionInfo struct {
	Name       string
	TmuxName   string // actual tmux session name (may have status prefix)
	Project    string
	Branch     string
	Status     string
	Annotation string
	StartedAt  time.Time
	UpdatedAt  time.Time
}
```

**Step 2: Update Show and Stop tests**

Replace `TestShow_OK`, `TestShow_NotFound`, `TestShow_NoOptions`, `TestShow_RunnerError`, `TestStop_OK`, `TestStop_NotFound`, `TestStop_KillError` in `internal/session/session_test.go`:

```go
func TestShow_OK(t *testing.T) {
	line := "myapp-feature\tmyapp-feature\tagent\trunning\t\t2024-01-01T00:00:00Z\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line},
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	info, err := svc.Show("myapp-feature")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Name != "myapp-feature" {
		t.Errorf("Name = %q, want myapp-feature", info.Name)
	}
	if info.TmuxName != "myapp-feature" {
		t.Errorf("TmuxName = %q, want myapp-feature", info.TmuxName)
	}
	if info.Status != semconv.StatusRunning {
		t.Errorf("Status = %q, want running", info.Status)
	}
	if info.StartedAt.IsZero() {
		t.Error("StartedAt should be non-zero")
	}
}

func TestShow_WaitingSession(t *testing.T) {
	// Session has prefix in tmux but canonical name is used for lookup.
	line := "⚡ myapp-feature\tmyapp-feature\tagent\twaiting\tneed input\t\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line},
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	info, err := svc.Show("myapp-feature")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Name != "myapp-feature" {
		t.Errorf("Name = %q, want myapp-feature (canonical)", info.Name)
	}
	if info.TmuxName != "⚡ myapp-feature" {
		t.Errorf("TmuxName = %q, want ⚡ myapp-feature", info.TmuxName)
	}
}

func TestShow_NotFound(t *testing.T) {
	r := &mockRunner{exitCode: 1} // list-sessions exit 1 = no sessions
	svc := newService(t, r)

	_, err := svc.Show("nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestShow_RunnerError(t *testing.T) {
	r := &mockRunner{exitCode: 1, err: errors.New("tmux exec failed")}
	svc := newService(t, r)

	_, err := svc.Show("myapp-feature")
	if err == nil {
		t.Fatal("expected error when runner fails")
	}
}

func TestStop_OK(t *testing.T) {
	line := "myapp-feature\tmyapp-feature\tagent\trunning\t\t\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line}, // list-sessions
		{exitCode: 0},               // kill-session
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	if err := svc.Stop("myapp-feature"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStop_WaitingSession(t *testing.T) {
	// Stop must kill the prefixed session name.
	line := "⚡ myapp-feature\tmyapp-feature\tagent\twaiting\t\t\n"
	r2 := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line}, // list-sessions
		{exitCode: 0},               // kill-session
	}}
	tc := tmux.NewClient(r2)
	svc := session.NewService(tc)

	if err := svc.Stop("myapp-feature"); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	// Verify kill-session targeted the prefixed name.
	killArgs := r2.calls[1]
	if killArgs[len(killArgs)-1] != "⚡ myapp-feature" {
		t.Errorf("kill-session target = %q, want ⚡ myapp-feature", killArgs[len(killArgs)-1])
	}
}

func TestStop_NotFound(t *testing.T) {
	r := &mockRunner{exitCode: 1} // list-sessions exit 1 = no sessions
	svc := newService(t, r)

	err := svc.Stop("nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStop_KillError(t *testing.T) {
	line := "myapp-feature\tmyapp-feature\tagent\trunning\t\t\n"
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line},                    // list-sessions
		{exitCode: 1, err: errors.New("kill failed")}, // kill-session
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.Stop("myapp-feature"); err == nil {
		t.Fatal("expected error when kill fails")
	}
}
```

Delete `TestShow_NoOptions` — a session without devenv options is not a managed session and `Show` correctly returns `ErrSessionNotFound`.

**Step 3: Run tests to verify they fail**

```bash
go test ./internal/session/...
```
Expected: FAIL.

**Step 4: Update session.Show and session.Stop**

Replace `Show` and `Stop` in `internal/session/session.go`:

```go
// Show returns the SessionInfo for a single named tmux session, looked up by canonical name.
// Returns ErrSessionNotFound if no agent session with that canonical name exists.
func (s *Service) Show(name string) (*SessionInfo, error) {
	records, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	for _, r := range records {
		if r.CanonicalName == name && r.SessionType == semconv.SessionTypeAgent {
			info := &SessionInfo{
				Name:       r.CanonicalName,
				TmuxName:   r.Name,
				Status:     r.Status,
				Annotation: r.Annotation,
			}
			if r.StartedAt != "" {
				info.StartedAt, _ = time.Parse(time.RFC3339, r.StartedAt)
			}
			return info, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, name)
}

// Stop kills the named tmux session, resolving the actual session name by canonical name.
// Returns ErrSessionNotFound if no agent session with that canonical name exists.
func (s *Service) Stop(name string) error {
	records, err := s.tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	actualName := ""
	for _, r := range records {
		if r.CanonicalName == name && r.SessionType == semconv.SessionTypeAgent {
			actualName = r.Name
			break
		}
	}
	if actualName == "" {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}
	if err := s.tmux.KillSession(actualName); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}
	return nil
}
```

**Step 5: Update cmd/session.go to use TmuxName for attach**

In `cmd/session.go`, the `sessionAttachCmd` uses `Show` to verify existence and then attaches. Since `Show` now returns a canonical name in `info.Name`, the attach must use `info.TmuxName`:

```go
var sessionAttachCmd = &cobra.Command{
	Use:   "attach <session>",
	Short: "Attach to an existing session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newSessionService()
		info, err := svc.Show(args[0]) // verify it exists
		if err != nil {
			return sessionErr(cmd, err)
		}
		return execTmuxAttach(info.TmuxName) // use actual tmux name for attach
	},
}
```

**Step 6: Run tests to verify they pass**

```bash
go test ./internal/session/... ./cmd/...
```
Expected: PASS.

**Step 7: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go cmd/session.go
git commit -m "feat: session Show/Stop resolve actual tmux name via canonical name lookup"
```

---

### Task 6: session.SetStatus — list-based canonical name lookup

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

**Step 1: Update SetStatus tests**

Replace `TestSetStatus_Running`, `TestSetStatus_Waiting`, `TestSetStatus_SuppressesError` in `internal/session/session_test.go`:

```go
func TestSetStatus_Running(t *testing.T) {
	// SetStatus("running") with canonical name resolves the prefixed actual name.
	line := "⚡ myapp-feature\tmyapp-feature\tagent\twaiting\t\t\n"
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line}, // list-sessions
		{exitCode: 0},               // set-option status
		{exitCode: 0},               // set-option annotation
		{exitCode: 0},               // rename-session (remove ⚡ prefix)
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.SetStatus("myapp-feature", "running", ""); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if len(r.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d: %v", len(r.calls), r.calls)
	}
	// Verify rename targeted the prefixed name.
	renameArgs := r.calls[3]
	if renameArgs[len(renameArgs)-2] != "⚡ myapp-feature" {
		t.Errorf("rename source = %q, want ⚡ myapp-feature", renameArgs[len(renameArgs)-2])
	}
	if renameArgs[len(renameArgs)-1] != "myapp-feature" {
		t.Errorf("rename target = %q, want myapp-feature", renameArgs[len(renameArgs)-1])
	}
}

func TestSetStatus_Waiting(t *testing.T) {
	// SetStatus("waiting") with canonical name adds the prefix.
	line := "myapp-feature\tmyapp-feature\tagent\trunning\t\t\n"
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 0, stdout: line}, // list-sessions
		{exitCode: 0},               // set-option status
		{exitCode: 0},               // set-option annotation
		{exitCode: 0},               // rename-session (add ⚡ prefix)
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.SetStatus("myapp-feature", "waiting", "Claude needs input"); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if len(r.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d", len(r.calls))
	}
}

func TestSetStatus_SuppressesError(t *testing.T) {
	// list-sessions fails — SetStatus suppresses the error and returns nil.
	r := &mockRunner{exitCode: 1, err: errors.New("tmux failed")}
	svc := newService(t, r)

	if err := svc.SetStatus("any-session", "running", ""); err != nil {
		t.Fatalf("SetStatus() should suppress errors: %v", err)
	}
}

func TestSetStatus_SessionNotFound(t *testing.T) {
	// Session with that canonical name does not exist — SetStatus is a no-op.
	r := &mockRunnerSequence{responses: []mockResponse{
		{exitCode: 1}, // list-sessions exit 1 = no sessions
	}}
	tc := tmux.NewClient(r)
	svc := session.NewService(tc)

	if err := svc.SetStatus("myapp-feature", "running", ""); err != nil {
		t.Fatalf("SetStatus() should suppress not-found: %v", err)
	}
	if len(r.calls) != 1 {
		t.Errorf("expected 1 call (list only), got %d", len(r.calls))
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/session/...
```
Expected: FAIL.

**Step 3: Update SetStatus**

Replace `SetStatus` in `internal/session/session.go`. Remove the `HasSession` probe entirely:

```go
// SetStatus transitions a session's status and updates the annotation.
// It resolves the actual tmux session name by canonical name.
// Errors are suppressed — this method always returns nil.
func (s *Service) SetStatus(name, status, annotation string) error {
	if name == "" {
		return nil
	}
	if status != semconv.StatusRunning && status != semconv.StatusWaiting {
		return nil
	}

	records, _ := s.tmux.ListSessions()
	actualName := ""
	for _, r := range records {
		if r.CanonicalName == name && r.SessionType == semconv.SessionTypeAgent {
			actualName = r.Name
			break
		}
	}
	if actualName == "" {
		return nil // session not found, suppress
	}

	_ = s.tmux.SetOption(actualName, semconv.TmuxOptionStatus, status)
	_ = s.tmux.SetOption(actualName, semconv.TmuxOptionAnnotation, annotation)

	hasPrefix := strings.HasPrefix(actualName, semconv.StatusPrefix)
	if status == semconv.StatusRunning && hasPrefix {
		_ = s.tmux.RenameSession(actualName, strings.TrimPrefix(actualName, semconv.StatusPrefix))
	} else if status != semconv.StatusRunning && !hasPrefix {
		_ = s.tmux.RenameSession(actualName, semconv.StatusPrefix+actualName)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/session/...
```
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat: SetStatus resolves actual session name via canonical name lookup"
```

---

### Task 7: tui/actions.go — set options on new shell sessions

**Files:**
- Modify: `internal/tui/actions.go`

Shell sessions keep the `~sh` suffix in their tmux name (for visibility in tmux session picker) but now store `@devenv_canonical_name = project-branch` and `@devenv_session_type = "shell"`.

**Step 1: Write the failing test**

In `internal/tui/actions_test.go` (or wherever shell action tests live), add a test verifying that the two new options are set after `NewSession`. If no test file exists for actions, skip this step and rely on the model_test.go integration.

Actually, the shell session creation is tested indirectly via `TestModel_refreshCmd_withTmuxClient` in Task 8. Skip a dedicated unit test here; the integration test in Task 8 will cover it.

**Step 2: Update shellAction in actions.go**

Find the shell session creation block in `internal/tui/actions.go` (around line 182–194). Add option-setting after `NewSession`:

Current code:
```go
shellName := semconv.ShellSessionName(project, branch)

// Create shell session if it doesn't exist.
exists, err := tmuxClient.HasSession(shellName)
if err != nil {
    return errMsg{err: err}
}
if !exists {
    if err := tmuxClient.NewSession(shellName, path); err != nil {
        return errMsg{err: err}
    }
}
```

Replace with:
```go
shellName := semconv.ShellSessionName(project, branch)
sessionName := semconv.SessionName(project, branch)

// Create shell session if it doesn't exist.
exists, err := tmuxClient.HasSession(shellName)
if err != nil {
    return errMsg{err: err}
}
if !exists {
    if err := tmuxClient.NewSession(shellName, path); err != nil {
        return errMsg{err: err}
    }
    _ = tmuxClient.SetOption(shellName, semconv.TmuxOptionCanonicalName, sessionName)
    _ = tmuxClient.SetOption(shellName, semconv.TmuxOptionSessionType, semconv.SessionTypeShell)
}
```

**Step 3: Run tests**

```bash
go test ./internal/tui/...
```
Expected: PASS (no new test was added; existing tests should still pass).

**Step 4: Commit**

```bash
git add internal/tui/actions.go
git commit -m "feat: set canonical name and session type options on new shell sessions"
```

---

### Task 8: tui/model.go and items.go — use structured records

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/items.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Update the refreshCmd test**

Replace `TestModel_refreshCmd_withTmuxClient` in `internal/tui/model_test.go`:

```go
func TestModel_refreshCmd_withTmuxClient(t *testing.T) {
	// Sessions: one agent session (waiting/prefixed), one shell session.
	agentLine := "⚡ myapp-feat\tmyapp-feat\tagent\twaiting\tneed input\t\n"
	shellLine := "myapp-feat~sh\tmyapp-feat\tshell\t\t\t\n"
	listOutput := agentLine + shellLine

	runner := &mockTmuxRunner{
		responses: []mockTmuxResponse{
			{stdout: listOutput, exitCode: 0}, // list-sessions (single call)
		},
	}
	client := tmux.NewClient(runner)

	m := Model{screen: screenList}
	m.list = newList(nil)
	m.tmuxClient = client

	cmd := m.refreshCmd()
	if cmd == nil {
		t.Fatal("refreshCmd() returned nil")
	}
	msg := cmd()
	items, ok := msg.(itemsMsg)
	if !ok {
		t.Fatalf("refreshCmd() produced %T, want itemsMsg", msg)
	}

	// Exactly 1 runner call — no per-session GetOption calls.
	if runner.idx != 1 {
		t.Errorf("runner.idx = %d, want 1 (single list-sessions call)", runner.idx)
	}

	// No item should have a Branch that ends with "~sh".
	for _, item := range items {
		if strings.HasSuffix(item.Branch, "~sh") {
			t.Errorf("shell session branch %q leaked into items", item.Branch)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/tui/...
```
Expected: FAIL — refreshCmd still calls GetOption per session.

**Step 3: Update refreshCmd in model.go**

In `internal/tui/model.go`, replace the tmux session loop inside `refreshCmd` (the block that currently iterates names from `tmuxClient.ListSessions()`):

Current code (roughly lines 319–338):
```go
if tmuxClient != nil {
    names, err := tmuxClient.ListSessions()
    if err == nil {
        for _, name := range names {
            if strings.HasSuffix(name, "~sh") {
                data.shellSessions[name] = true
                continue
            }
            status, _ := tmuxClient.GetOption(name, semconv.TmuxOptionStatus)
            annotation, _ := tmuxClient.GetOption(name, semconv.TmuxOptionAnnotation)
            key := strings.TrimPrefix(name, semconv.StatusPrefix)
            data.agentSessions[key] = agentInfo{
                status:     status,
                annotation: annotation,
            }
        }
    }
}
```

Replace with:
```go
if tmuxClient != nil {
    records, err := tmuxClient.ListSessions()
    if err == nil {
        for _, r := range records {
            switch r.SessionType {
            case semconv.SessionTypeShell:
                data.shellSessions[r.CanonicalName] = true
            case semconv.SessionTypeAgent:
                data.agentSessions[r.CanonicalName] = agentInfo{
                    status:     r.Status,
                    annotation: r.Annotation,
                }
            }
        }
    }
}
```

Remove the now-unused `strings` import if it's no longer referenced elsewhere in `model.go`.

**Step 4: Update items.go**

In `internal/tui/items.go`, the `buildItems` function references `shellName` for the `HasShell` lookup. Change it to use `sessionName`:

Find this block (around line 73–82):
```go
sessionName := semconv.SessionName(wt.project, wt.branch)
shellName := semconv.ShellSessionName(wt.project, wt.branch)

item := Item{
    Project:  wt.project,
    Branch:   wt.branch,
    Path:     wt.path,
    HasShell: data.shellSessions[shellName],
    IsMain:   data.cloneDirs[wt.project] == wt.path,
}
```

Replace with:
```go
sessionName := semconv.SessionName(wt.project, wt.branch)

item := Item{
    Project:  wt.project,
    Branch:   wt.branch,
    Path:     wt.path,
    HasShell: data.shellSessions[sessionName],
    IsMain:   data.cloneDirs[wt.project] == wt.path,
}
```

Update the comment on the `shellSessions` field in `refreshResult` in `items.go`:
```go
shellSessions map[string]bool // keyed by canonical session name (project-branch)
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/tui/...
```
Expected: PASS.

**Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/items.go internal/tui/model_test.go
git commit -m "refactor: tui refresh uses structured list-sessions, no per-session GetOption"
```

---

### Task 9: verify coverage and full test suite

**Step 1: Run full test suite**

```bash
make test
```
Expected: all tests PASS.

**Step 2: Run coverage check**

```bash
make coverage
```
Expected: aggregate coverage ≥ 80%. If below threshold, add tests to the packages that dropped.

**Step 3: Run integration tests**

```bash
make test-integration
```
Expected: PASS.

**Step 4: Commit any coverage fixes if needed**

```bash
git add <files>
git commit -m "test: add coverage to meet 80% threshold"
```
