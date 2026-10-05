//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func decodeDescribe(t *testing.T, raw string) describeOutput {
	t.Helper()
	var out describeOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode describe JSON: %v\nraw: %q", err, raw)
	}
	return out
}

func TestDescribeManagedWorkspace(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "describe", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	out := decodeDescribe(t, result.stdout)
	if out.Workspace.Mode != "managed" || out.Workspace.Root == "" {
		t.Fatalf("workspace = %+v, want managed with a root", out.Workspace)
	}
	if len(out.Contexts) == 0 || !out.Contexts[0].Default {
		t.Fatalf("contexts = %+v, want a default context", out.Contexts)
	}
	if out.ExitCodes["busy"] != exitTemporary || out.ExitCodes["unavailable"] != exitUnavailable {
		t.Fatalf("exit codes = %+v", out.ExitCodes)
	}
	if len(out.Environment) == 0 {
		t.Fatalf("environment contract is empty")
	}
	if out.OfficialCF != fakeCF {
		t.Fatalf("official_cf = %q, want %q", out.OfficialCF, fakeCF)
	}
}

func TestDescribeUnresolvedWorkspace(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "describe", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	out := decodeDescribe(t, result.stdout)
	if out.Workspace.Mode != "unresolved" {
		t.Fatalf("mode = %q, want unresolved", out.Workspace.Mode)
	}
	if len(out.Contexts) != 0 {
		t.Fatalf("contexts = %+v, want empty", out.Contexts)
	}
}

func TestDescribeExternalCFHome(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	externalHome := filepath.Join(t.TempDir(), "external")
	t.Setenv("CF_HOME", externalHome)

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "describe", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	out := decodeDescribe(t, result.stdout)
	if out.Workspace.Mode != "external" || out.Workspace.CFHome != externalHome {
		t.Fatalf("workspace = %+v, want external %q", out.Workspace, externalHome)
	}
}

func TestDescribeHumanOutput(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "describe"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	if !strings.Contains(result.stdout, "Workspace:") || !strings.Contains(result.stdout, "--json") {
		t.Fatalf("stdout = %q", result.stdout)
	}
}

func TestDescribeRejectsPositionalArgs(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())

	result := runFromDirectory(t, t.TempDir(), []string{"cfs", "describe", "extra"})
	if result.code != exitUsage {
		t.Fatalf("exit code = %d, want %d", result.code, exitUsage)
	}
}
