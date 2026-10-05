//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestContextCreateJSON(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "context", "create", "prod", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out createResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "created" || out.Context != "prod" || out.Workspace != canonicalTestPath(t, root) || out.CFHome == "" {
		t.Fatalf("result = %+v", out)
	}
}

func TestContextRemoveJSON(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	if r := runFromDirectory(t, root, []string{"cfs", "context", "create", "prod"}); r.code != 0 {
		t.Fatalf("create exit = %d, stderr = %q", r.code, r.stderr)
	}
	result := runFromDirectory(t, root, []string{"cfs", "context", "remove", "prod", "--yes", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out removeResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "removed" || out.Context != "prod" || out.TrashPath == "" || out.Warning == "" {
		t.Fatalf("result = %+v", out)
	}
}

func TestResetJSONNoState(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "reset", "--yes", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out resetResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "none" || out.Workspace != canonicalTestPath(t, root) || out.TrashPath != "" {
		t.Fatalf("result = %+v", out)
	}
}

func TestResetJSONMovesState(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	// Create managed state so reset has something to move.
	if r := runFromDirectory(t, root, []string{"cf", "apps"}); r.code != 0 {
		t.Fatalf("seed cf exit = %d, stderr = %q", r.code, r.stderr)
	}
	result := runFromDirectory(t, root, []string{"cfs", "reset", "--yes", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out resetResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "reset" || out.TrashPath == "" || out.Warning == "" {
		t.Fatalf("result = %+v", out)
	}
}

func TestImportJSON(t *testing.T) {
	fakeCF := writeTargetAwareFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	if err := os.Chmod(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	configureTestEnvironment(t, fakeCF, stateRoot)
	globalHome := t.TempDir()
	t.Setenv("HOME", globalHome)
	writeCFConfig(t, globalHome, []byte(`{"Target":"https://api.example.com"}`))
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "import", "--yes", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out importResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "imported" || out.Workspace != canonicalTestPath(t, root) || out.CFHome == "" || out.Context == "" {
		t.Fatalf("result = %+v", out)
	}
}

func TestMutationJSONStdoutIsCleanJSON(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "context", "create", "prod", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	// stdout must be JSON only — no human prose leaking in.
	if !strings.HasPrefix(strings.TrimSpace(result.stdout), "{") {
		t.Fatalf("stdout is not pure JSON: %q", result.stdout)
	}
}
