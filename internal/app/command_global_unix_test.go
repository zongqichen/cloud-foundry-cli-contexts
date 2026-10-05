//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalRunsOutsideWorkspace(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	// A plain directory with no .cfs.toml: a normal `cf apps` fails closed,
	// but `cfs global` must escape isolation and run against the global home.
	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global", "apps"})

	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if got := outputValue(result.stdout, "ARGS"); got != "apps" {
		t.Fatalf("ARGS = %q, want %q", got, "apps")
	}
	if !strings.Contains(result.stderr, "global CF home") {
		t.Fatalf("stderr = %q, want global target notice", result.stderr)
	}
}

func TestGlobalPropagatesExitCode(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global", "exit-42"})
	if result.code != 42 {
		t.Fatalf("exit code = %d, want 42; stderr = %q", result.code, result.stderr)
	}
}

func TestGlobalRequiresCommand(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global"})
	if result.code != exitUsage {
		t.Fatalf("exit code = %d, want %d", result.code, exitUsage)
	}
	if !strings.Contains(result.stderr, "requires a CF command") {
		t.Fatalf("stderr = %q", result.stderr)
	}
	if strings.Contains(result.stdout, "ARGS=") {
		t.Fatalf("official CLI unexpectedly ran: %q", result.stdout)
	}
}

func TestGlobalRespectsExplicitCFHome(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	externalHome := filepath.Join(t.TempDir(), "external")
	t.Setenv("CF_HOME", externalHome)

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global", "target"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if got := outputValue(result.stdout, "CF_HOME"); got != externalHome {
		t.Fatalf("CF_HOME = %q, want %q", got, externalHome)
	}
	if !strings.Contains(result.stderr, externalHome) {
		t.Fatalf("stderr = %q, want notice naming %q", result.stderr, externalHome)
	}
}

func TestGlobalHelpDoesNotRunCF(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global", "--help"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if !strings.Contains(result.stdout, "escape the current workspace") {
		t.Fatalf("stdout = %q, want help text", result.stdout)
	}
	if strings.Contains(result.stdout, "ARGS=") {
		t.Fatalf("official CLI unexpectedly ran: %q", result.stdout)
	}
}

func TestGlobalBypassesManagedWorkspace(t *testing.T) {
	fakeCF := writeFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)
	root := markerWorkspace(t)

	// Standing inside a workspace, a normal cf run uses the managed home.
	managed := runFromDirectory(t, root, []string{"cf", "apps"})
	if managed.code != 0 {
		t.Fatalf("managed cf exit = %d, stderr = %q", managed.code, managed.stderr)
	}
	managedHome := outputValue(managed.stdout, "CF_HOME")
	if managedHome == "" || !strings.HasPrefix(managedHome, stateRoot) {
		t.Fatalf("managed CF_HOME = %q, want under %q", managedHome, stateRoot)
	}

	// From the same workspace, global must escape the managed home.
	global := runFromDirectory(t, root, []string{"cfs", "global", "apps"})
	if global.code != 0 {
		t.Fatalf("cfs global exit = %d, stderr = %q", global.code, global.stderr)
	}
	if got := outputValue(global.stdout, "CF_HOME"); got == managedHome {
		t.Fatalf("cfs global used the managed home %q, want the global home", got)
	}
}

func TestGlobalStripsActiveContextMarker(t *testing.T) {
	fakeCF := writeActiveContextEchoFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	t.Setenv("CFS_ACTIVE_CONTEXT", "ctx-should-be-stripped")

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "global", "apps"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if got := outputValue(result.stdout, "CFS_ACTIVE_CONTEXT"); got != "" {
		t.Fatalf("CFS_ACTIVE_CONTEXT leaked to cf: %q", got)
	}
}

func writeActiveContextEchoFakeCF(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cf-real")
	script := "#!/bin/sh\n" +
		"printf 'CF_HOME=%s\\n' \"$CF_HOME\"\n" +
		"printf 'CFS_ACTIVE_CONTEXT=%s\\n' \"$CFS_ACTIVE_CONTEXT\"\n" +
		"printf 'ARGS=%s\\n' \"$*\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
