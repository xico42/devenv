# `internal/envtemplate` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the `internal/envtemplate` package — a pure library with `DeterministicPort`, `ParseEnvFile`, and `Process` used by the worktree and session commands.

**Architecture:** One package (`internal/envtemplate`), one source file, one test file. No CLI wiring — this is consumed by `cmd/worktree.go` and `cmd/session.go` in separate branches. Uses `hash/fnv` and `text/template` from stdlib only.

**Tech Stack:** Go 1.26, `hash/fnv` (stdlib), `text/template` (stdlib), `os` (stdlib)

---

### Task 1: `DeterministicPort`

**Files:**
- Create: `internal/envtemplate/envtemplate.go`
- Create: `internal/envtemplate/envtemplate_test.go`

**Step 1: Write the failing tests**

Create `internal/envtemplate/envtemplate_test.go`:

```go
package envtemplate_test

import (
	"testing"

	"github.com/xico42/devenv/internal/envtemplate"
)

func TestDeterministicPort(t *testing.T) {
	t.Run("idempotent", func(t *testing.T) {
		p1 := envtemplate.DeterministicPort("myapp", "feature", "api")
		p2 := envtemplate.DeterministicPort("myapp", "feature", "api")
		if p1 != p2 {
			t.Errorf("not idempotent: %d != %d", p1, p2)
		}
	})

	t.Run("in range 10000-59999", func(t *testing.T) {
		p := envtemplate.DeterministicPort("myapp", "feature", "api")
		if p < 10000 || p > 59999 {
			t.Errorf("port %d out of range 10000-59999", p)
		}
	})

	t.Run("different name gives different port", func(t *testing.T) {
		p1 := envtemplate.DeterministicPort("myapp", "feature", "api")
		p2 := envtemplate.DeterministicPort("myapp", "feature", "db")
		if p1 == p2 {
			t.Errorf("same port %d for different names", p1)
		}
	})

	t.Run("null-byte separation prevents ambiguity", func(t *testing.T) {
		// ("ab","cd","x") must differ from ("a","bcd","x")
		p1 := envtemplate.DeterministicPort("ab", "cd", "x")
		p2 := envtemplate.DeterministicPort("a", "bcd", "x")
		if p1 == p2 {
			t.Errorf("null-byte separation failed: both hashed to %d", p1)
		}
	})
}
```

**Step 2: Run to verify it fails**

```bash
go test ./internal/envtemplate/...
```

Expected: compile error — package does not exist yet.

**Step 3: Create `internal/envtemplate/envtemplate.go` with minimal implementation**

```go
package envtemplate

import "hash/fnv"

// DeterministicPort returns a stable port for the given project/branch/name.
// Uses FNV-1a 32-bit hash with null-byte separators. Range: 10000–59999.
func DeterministicPort(project, branch, name string) int {
	key := project + "\x00" + branch + "\x00" + name
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()%50000) + 10000
}
```

**Step 4: Run to verify tests pass**

```bash
go test ./internal/envtemplate/...
```

Expected: PASS (4 tests).

**Step 5: Commit**

```bash
git add internal/envtemplate/
git commit -m "feat(envtemplate): add DeterministicPort"
```

---

### Task 2: `ParseEnvFile`

**Files:**
- Modify: `internal/envtemplate/envtemplate.go`
- Modify: `internal/envtemplate/envtemplate_test.go`

**Step 1: Write the failing tests**

Add to `internal/envtemplate/envtemplate_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/envtemplate"
)

// helper — add near the top of the file, outside any Test func
func writeTempEnv(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(f, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return f
}

func TestParseEnvFile(t *testing.T) {
	t.Run("normal key-value pairs", func(t *testing.T) {
		f := writeTempEnv(t, "KEY=value\nFOO=bar\n")
		got, err := envtemplate.ParseEnvFile(f)
		if err != nil {
			t.Fatalf("ParseEnvFile() error = %v", err)
		}
		if got["KEY"] != "value" {
			t.Errorf("KEY = %q, want %q", got["KEY"], "value")
		}
		if got["FOO"] != "bar" {
			t.Errorf("FOO = %q, want %q", got["FOO"], "bar")
		}
	})

	t.Run("skips comments and blank lines", func(t *testing.T) {
		f := writeTempEnv(t, "# comment\n\nKEY=value\n")
		got, err := envtemplate.ParseEnvFile(f)
		if err != nil {
			t.Fatalf("ParseEnvFile() error = %v", err)
		}
		if len(got) != 1 {
			t.Errorf("len = %d, want 1; got %v", len(got), got)
		}
	})

	t.Run("value with equals sign splits on first only", func(t *testing.T) {
		f := writeTempEnv(t, "URL=postgres://user:pass@host/db?opt=val\n")
		got, err := envtemplate.ParseEnvFile(f)
		if err != nil {
			t.Fatalf("ParseEnvFile() error = %v", err)
		}
		want := "postgres://user:pass@host/db?opt=val"
		if got["URL"] != want {
			t.Errorf("URL = %q, want %q", got["URL"], want)
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		_, err := envtemplate.ParseEnvFile("/nonexistent/path/.env")
		if err == nil {
			t.Fatal("ParseEnvFile() error = nil, want error for missing file")
		}
	})
}
```

**Step 2: Run to verify it fails**

```bash
go test ./internal/envtemplate/...
```

Expected: compile error — `ParseEnvFile` undefined.

**Step 3: Add `ParseEnvFile` to `envtemplate.go`**

Add these imports and the function (update the import block at the top of the file):

```go
import (
	"fmt"
	"hash/fnv"
	"os"
	"strings"
)

// ParseEnvFile reads a .env file and returns key-value pairs.
// Skips blank lines and lines starting with "#".
// Splits on the first "=" only — values may contain "=".
func ParseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading env file: %w", err)
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		result[line[:idx]] = line[idx+1:]
	}
	return result, nil
}
```

**Step 4: Run to verify tests pass**

```bash
go test ./internal/envtemplate/...
```

Expected: PASS (8 tests).

**Step 5: Commit**

```bash
git add internal/envtemplate/envtemplate.go internal/envtemplate/envtemplate_test.go
git commit -m "feat(envtemplate): add ParseEnvFile"
```

---

### Task 3: `Process`

**Files:**
- Modify: `internal/envtemplate/envtemplate.go`
- Modify: `internal/envtemplate/envtemplate_test.go`

**Step 1: Write the failing tests**

Add to the imports in `envtemplate_test.go`:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xico42/devenv/internal/envtemplate"
)
```

Add these test cases to `envtemplate_test.go`:

```go
func TestProcess(t *testing.T) {
	ctx := envtemplate.EnvTemplateContext{
		Project:      "myapp",
		Branch:       "feature",
		WorktreePath: "/home/user/projects/myapp__worktrees/feature",
		SessionName:  "myapp-feature",
	}

	t.Run("context fields render", func(t *testing.T) {
		tmpl := "{{ .Project }} {{ .Branch }} {{ .WorktreePath }} {{ .SessionName }}"
		got, err := envtemplate.Process(tmpl, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		for _, want := range []string{"myapp", "feature", "/home/user/projects/myapp__worktrees/feature", "myapp-feature"} {
			if !strings.Contains(got, want) {
				t.Errorf("output missing %q\ngot:\n%s", want, got)
			}
		}
	})

	t.Run("port matches DeterministicPort", func(t *testing.T) {
		got, err := envtemplate.Process(`{{ port "api" }}`, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		expected := fmt.Sprintf("%d", envtemplate.DeterministicPort("myapp", "feature", "api"))
		if !strings.Contains(got, expected) {
			t.Errorf("port output %q does not contain %q", got, expected)
		}
	})

	t.Run("port same name same value in output", func(t *testing.T) {
		got, err := envtemplate.Process(`A={{ port "api" }} B={{ port "api" }}`, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		expected := fmt.Sprintf("%d", envtemplate.DeterministicPort("myapp", "feature", "api"))
		if strings.Count(got, expected) < 2 {
			t.Errorf("expected port %s to appear twice in:\n%s", expected, got)
		}
	})

	t.Run("env returns set var", func(t *testing.T) {
		t.Setenv("DEVENV_TEST_SECRET", "mysecret")
		got, err := envtemplate.Process(`{{ env "DEVENV_TEST_SECRET" "default" }}`, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		if !strings.Contains(got, "mysecret") {
			t.Errorf("output missing env var value\ngot:\n%s", got)
		}
	})

	t.Run("env unset var uses default", func(t *testing.T) {
		got, err := envtemplate.Process(`{{ env "DEVENV_UNSET_VAR_12345" "mydefault" }}`, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		if !strings.Contains(got, "mydefault") {
			t.Errorf("output missing default value\ngot:\n%s", got)
		}
	})

	t.Run("env unset var no default returns empty string", func(t *testing.T) {
		got, err := envtemplate.Process(`A={{ env "DEVENV_UNSET_VAR_12345" }}B`, "test", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		if !strings.Contains(got, "A=B") {
			t.Errorf("expected empty string for unset var, got:\n%s", got)
		}
	})

	t.Run("header present in output", func(t *testing.T) {
		got, err := envtemplate.Process("KEY=value", "repo-local", ctx)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		if !strings.HasPrefix(got, "# Generated by devenv") {
			t.Errorf("output missing generated header\ngot:\n%s", got)
		}
		if !strings.Contains(got, "repo-local") {
			t.Errorf("output missing source in header\ngot:\n%s", got)
		}
	})

	t.Run("template syntax error returns error", func(t *testing.T) {
		_, err := envtemplate.Process("{{ .Invalid }", "test", ctx)
		if err == nil {
			t.Fatal("Process() error = nil, want error for syntax error")
		}
	})
}
```

**Step 2: Run to verify it fails**

```bash
go test ./internal/envtemplate/...
```

Expected: compile error — `EnvTemplateContext` and `Process` undefined.

**Step 3: Add `EnvTemplateContext` and `Process` to `envtemplate.go`**

Update the import block to add `bytes`, `fmt` (already present), `text/template`:

```go
import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"text/template"
)
```

Add after `ParseEnvFile`:

```go
// EnvTemplateContext holds the values available to .env templates.
type EnvTemplateContext struct {
	Project      string // project name from config (e.g. "myapp")
	Branch       string // branch name (e.g. "feature")
	WorktreePath string // absolute path to worktree
	SessionName  string // "<project>-<branch>"
}

// Process renders a .env.template string with the given context and returns
// the full .env file content, including a generated header comment.
// source describes where the template came from (e.g. "repo-local", "/path/to/file").
func Process(templateContent, source string, ctx EnvTemplateContext) (string, error) {
	funcMap := template.FuncMap{
		"port": func(name string) int {
			return DeterministicPort(ctx.Project, ctx.Branch, name)
		},
		"env": func(args ...string) string {
			if len(args) == 0 {
				return ""
			}
			if v := os.Getenv(args[0]); v != "" {
				return v
			}
			if len(args) > 1 {
				return args[1]
			}
			return ""
		},
	}

	tmpl, err := template.New("env").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Generated by devenv — do not edit (regenerate with: devenv worktree env %s %s)\n", ctx.Project, ctx.Branch)
	fmt.Fprintf(&buf, "# Template: %s\n\n", source)

	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
```

**Step 4: Run to verify tests pass**

```bash
go test ./internal/envtemplate/...
```

Expected: PASS (16 tests).

**Step 5: Commit**

```bash
git add internal/envtemplate/envtemplate.go internal/envtemplate/envtemplate_test.go
git commit -m "feat(envtemplate): add EnvTemplateContext and Process"
```

---

### Task 4: Coverage check

**Step 1: Run coverage**

```bash
make coverage
```

Expected: aggregate coverage ≥ 80%. If it fails, add missing test cases for any uncovered branches in `envtemplate.go` before proceeding.

**Step 2: Run full test suite**

```bash
make test
```

Expected: all packages PASS.

**Step 3: Commit (if coverage fixes were needed)**

If you had to add tests in the previous steps:

```bash
git add internal/envtemplate/envtemplate_test.go
git commit -m "test(envtemplate): add coverage for missing branches"
```
