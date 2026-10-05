//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"encoding/json"
	"testing"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/workspace"
)

func TestContextListTargetsJSON(t *testing.T) {
	fakeCF := writeFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)
	root := markerWorkspace(t)

	if r := runFromDirectory(t, root, []string{"cfs", "context", "create", "prod"}); r.code != 0 {
		t.Fatalf("create exit = %d, stderr = %q", r.code, r.stderr)
	}
	ws, err := workspace.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := store.New(stateRoot).ContextForName(ws, "prod")
	if err != nil {
		t.Fatal(err)
	}
	writeCFConfig(t, ctx.CFHome, []byte(`{"Target":"https://api.example.com","OrganizationFields":{"Name":"org1"},"SpaceFields":{"Name":"space1"}}`))

	result := runFromDirectory(t, root, []string{"cfs", "context", "list", "--targets", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	var payload struct {
		Contexts []contextListItem `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode: %v\nraw: %q", err, result.stdout)
	}

	var prod *contextListItem
	for i := range payload.Contexts {
		if payload.Contexts[i].Name == "prod" {
			prod = &payload.Contexts[i]
		}
	}
	if prod == nil {
		t.Fatalf("prod context missing: %+v", payload.Contexts)
	}
	if prod.Target == nil {
		t.Fatalf("prod target missing")
	}
	if prod.Target.API != "https://api.example.com" || prod.Target.Org != "org1" || prod.Target.Space != "space1" {
		t.Fatalf("prod target = %+v", prod.Target)
	}

	// The implicit default context has no stored config, so it must have no target.
	for i := range payload.Contexts {
		item := payload.Contexts[i]
		if item.Name != "prod" && item.Target != nil {
			t.Fatalf("context %q unexpectedly has a target: %+v", item.Name, item.Target)
		}
	}
}

func TestContextListWithoutTargetsOmitsTarget(t *testing.T) {
	fakeCF := writeFakeCF(t)
	configureTestEnvironment(t, fakeCF, t.TempDir())
	root := markerWorkspace(t)

	result := runFromDirectory(t, root, []string{"cfs", "context", "list", "--json"})
	if result.code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.code, result.stderr)
	}
	// Without --targets, no "target" key should appear at all.
	if contains := jsonHasKey(result.stdout, "target"); contains {
		t.Fatalf("list without --targets leaked a target field: %q", result.stdout)
	}
}

func jsonHasKey(raw, key string) bool {
	var generic any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return false
	}
	return findKey(generic, key)
}

func findKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for k, v := range typed {
			if k == key || findKey(v, key) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if findKey(item, key) {
				return true
			}
		}
	}
	return false
}
