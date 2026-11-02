package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/envvar"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/runner"
)

const applicationTestChild = "CFS_APPLICATION_TEST_CHILD"

func TestMain(m *testing.M) {
	os.Exit(runAppTests(m))
}

func runAppTests(m *testing.M) (code int) {
	root, err := os.MkdirTemp("", "cfs-app-tests-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfs app tests: create temporary root: %v\n", err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintf(os.Stderr, "cfs app tests: remove temporary root: %v\n", err)
			if code == 0 {
				code = 1
			}
		}
	}()

	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cfs app tests: resolve temporary root: %v\n", err)
		return 1
	}
	home := filepath.Join(canonicalRoot, "home")
	if err := os.Mkdir(home, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "cfs app tests: create temporary home: %v\n", err)
		return 1
	}

	environment := []struct {
		name  string
		value string
	}{
		{name: "HOME", value: home},
		{name: "USERPROFILE", value: home},
		{name: envvar.ConfigFile, value: filepath.Join(canonicalRoot, "config", "config.json")},
		{name: envvar.StateHome, value: filepath.Join(canonicalRoot, "state")},
		{name: envvar.CFHome},
		{name: envvar.CFPluginHome},
		{name: envvar.CFTrace},
		{name: envvar.Disable},
		{name: envvar.ActiveContext},
		{name: envvar.WorkspaceRoot},
	}
	for _, variable := range environment {
		if err := os.Setenv(variable.name, variable.value); err != nil {
			fmt.Fprintf(os.Stderr, "cfs app tests: set %s: %v\n", variable.name, err)
			return 1
		}
	}

	return m.Run()
}

func TestApplicationTestsIgnoreCallerHome(t *testing.T) {
	if os.Getenv(applicationTestChild) == "1" {
		assertApplicationTestIsolation(t)
		return
	}

	callerHome := t.TempDir()
	configDir := filepath.Join(callerHome, ".cf")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"Target":"https://synthetic.invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestApplicationTestsIgnoreCallerHome$")
	command.Env = runner.ReplaceEnv(os.Environ(), map[string]string{
		applicationTestChild: "1",
		"HOME":               callerHome,
		"USERPROFILE":        callerHome,
		envvar.Disable:       "1",
	})
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child test timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("child test failed: %v\n%s", err, output)
	}
}

func assertApplicationTestIsolation(t *testing.T) {
	t.Helper()
	_, available, err := globalTargetAvailable()
	if err != nil {
		t.Errorf("globalTargetAvailable() error = %v", err)
	} else if available {
		t.Errorf("globalTargetAvailable() = true, want false")
	}

	t.Setenv(envvar.ConfigFile, filepath.Join(t.TempDir(), "missing.json"))
	var stderr bytes.Buffer
	code := Run(Options{
		Args:   []string{"cfs", "-c", "qa", "apps"},
		Stdin:  bytes.NewReader(nil),
		Stdout: io.Discard,
		Stderr: &stderr,
	})
	if code != exitUnavailable {
		t.Errorf("Run() = %d, want %d; stderr = %q", code, exitUnavailable, stderr.String())
	}
}
