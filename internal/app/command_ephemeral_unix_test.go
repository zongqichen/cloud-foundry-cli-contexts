//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextCreateEphemeralJSON(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "context", "create", "--ephemeral", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var out createResult
	if err := json.Unmarshal([]byte(result.stdout), &out); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}
	if out.Action != "created" || !out.Ephemeral {
		t.Fatalf("result = %+v", out)
	}
	if !strings.HasPrefix(out.Context, "eph-") {
		t.Fatalf("ephemeral name = %q, want eph- prefix", out.Context)
	}
}

func TestContextCreateRequiresNameWithoutEphemeral(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "context", "create"})
	if result.code != exitUsage {
		t.Fatalf("exit code = %d, want %d; stderr = %q", result.code, exitUsage, result.stderr)
	}
}

func TestGCReapsIdleEphemeral(t *testing.T) {
	fakeCF := writeFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)
	t.Setenv("CFS_EPHEMERAL_TTL", "0") // any idle time counts as expired
	root := markerWorkspace(t)

	eph := runFromDirectory(t, root, []string{"cfs", "context", "create", "--ephemeral", "--json"})
	if eph.code != 0 {
		t.Fatalf("create ephemeral exit = %d, stderr = %q", eph.code, eph.stderr)
	}
	var created createResult
	if err := json.Unmarshal([]byte(eph.stdout), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r := runFromDirectory(t, root, []string{"cfs", "context", "create", "keep"}); r.code != 0 {
		t.Fatalf("create keep exit = %d, stderr = %q", r.code, r.stderr)
	}

	gc := runFromDirectory(t, root, []string{"cfs", "gc", "--json"})
	if gc.code != 0 {
		t.Fatalf("gc exit = %d, stderr = %q", gc.code, gc.stderr)
	}
	var payload struct {
		Contexts []gcEntry `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(gc.stdout), &payload); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, gc.stdout)
	}

	var sawEphemeral bool
	for _, item := range payload.Contexts {
		if item.Name == created.Context {
			sawEphemeral = true
			if item.Reason != "ephemeral-expired" {
				t.Fatalf("ephemeral reason = %q, want ephemeral-expired", item.Reason)
			}
		}
		if item.Name == "keep" {
			t.Fatalf("non-ephemeral context in a live workspace was reaped: %+v", item)
		}
	}
	if !sawEphemeral {
		t.Fatalf("ephemeral context not reaped: %+v", payload.Contexts)
	}
}

func TestGCKeepsFreshEphemeral(t *testing.T) {
	fakeCF := writeFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)
	t.Setenv("CFS_EPHEMERAL_TTL", "24h")
	root := markerWorkspace(t)

	eph := runFromDirectory(t, root, []string{"cfs", "context", "create", "--ephemeral", "--json"})
	if eph.code != 0 {
		t.Fatalf("create ephemeral exit = %d, stderr = %q", eph.code, eph.stderr)
	}
	gc := runFromDirectory(t, root, []string{"cfs", "gc", "--json"})
	if gc.code != 0 {
		t.Fatalf("gc exit = %d, stderr = %q", gc.code, gc.stderr)
	}
	var payload struct {
		Contexts []gcEntry `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(gc.stdout), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Contexts) != 0 {
		t.Fatalf("fresh ephemeral should not be reaped within TTL: %+v", payload.Contexts)
	}
}
