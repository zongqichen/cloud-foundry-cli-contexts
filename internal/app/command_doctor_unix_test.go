//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/config"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/executable"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/workspace"
)

func TestDoctorDescribesLocalTargetWithoutClaimingAuthentication(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		status  checkStatus
		message string
	}{
		{
			name:    "complete target",
			config:  `{"Target":"https://api.example.com","OrganizationFields":{"Name":"org1"},"SpaceFields":{"Name":"space1"}}`,
			status:  checkPass,
			message: "local target configured; authentication not verified",
		},
		{
			name:    "missing organization",
			config:  `{"Target":"https://api.example.com","SpaceFields":{"Name":"space1"}}`,
			status:  checkWarn,
			message: "local login configured; authentication not verified; choose org/space with 'cf target -o <org> -s <space>'",
		},
		{
			name:    "missing space",
			config:  `{"Target":"https://api.example.com","OrganizationFields":{"Name":"org1"}}`,
			status:  checkWarn,
			message: "local login configured; authentication not verified; choose org/space with 'cf target -o <org> -s <space>'",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := prepareDoctorTarget(t, []byte(test.config))

			text := runFromDirectory(t, root, []string{"cfs", "doctor"})
			if text.code != exitOK {
				t.Fatalf("text exit code = %d; stderr = %q", text.code, text.stderr)
			}
			assertDoctorTargetText(t, text.stdout, test.status, test.message)

			result := runFromDirectory(t, root, []string{"cfs", "doctor", "--json"})
			if result.code != exitOK {
				t.Fatalf("JSON exit code = %d; stderr = %q", result.code, result.stderr)
			}
			var output doctorOutput
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatalf("decode doctor JSON: %v; output = %q", err, result.stdout)
			}
			for _, check := range output.Checks {
				if check.Name == "workspace-target" {
					if check.Status != test.status || check.Message != test.message {
						t.Fatalf("workspace target check = %#v, want status %q and message %q", check, test.status, test.message)
					}
					return
				}
			}
			t.Fatalf("doctor JSON has no workspace-target check: %s", result.stdout)
		})
	}
}

func prepareDoctorTarget(t *testing.T, rawConfig []byte) string {
	t.Helper()
	fakeCF := writeTargetAwareFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(cfg.ShimDir, executable.Name("cf"))
	if err := os.Symlink(self, shim); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", cfg.ShimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := markerWorkspace(t)
	ws, err := workspace.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	stateStore := store.New(stateRoot)
	ctx, err := stateStore.ContextFor(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateStore.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	writeCFConfig(t, ctx.CFHome, rawConfig)
	return root
}

func assertDoctorTargetText(t *testing.T, output string, status checkStatus, message string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "workspace-target") {
			if !strings.HasPrefix(line, strings.ToUpper(string(status))) || !strings.HasSuffix(line, message) {
				t.Fatalf("workspace target line = %q, want status %q and message %q", line, status, message)
			}
			return
		}
	}
	t.Fatalf("doctor text has no workspace-target check: %s", output)
}
