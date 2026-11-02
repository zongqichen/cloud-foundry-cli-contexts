package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/lock"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/workspace"
	"golang.org/x/sys/unix"
)

func TestShimPreservesConfigurationCreatedWhileWaitingForLock(t *testing.T) {
	fakeCF := writeFakeCF(t)
	stateRoot := canonicalTestPath(t, t.TempDir())
	configureTestEnvironment(t, fakeCF, stateRoot)
	t.Setenv("CFS_LOCK_TIMEOUT", "5s")
	globalHome := t.TempDir()
	t.Setenv("HOME", globalHome)
	writeCFConfig(t, globalHome, []byte(`{"Target":"https://synthetic-global.invalid"}`))
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
	held, err := lock.Acquire(ctx.LockPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	lockHeld := true
	t.Cleanup(func() {
		if lockHeld {
			_ = held.Release()
		}
	})

	watch, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(watch) })
	if _, err := unix.InotifyAddWatch(watch, ctx.LockPath, unix.IN_OPEN); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	result := make(chan commandResult, 1)
	go func() {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(Options{Args: []string{"cf", "version"}, Stdout: &stdout, Stderr: &stderr})
		result <- commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}()

	events := []unix.PollFd{{Fd: int32(watch), Events: unix.POLLIN}}
	ready, err := unix.Poll(events, 5_000)
	if err != nil {
		t.Fatal(err)
	}
	if ready != 1 || events[0].Revents&unix.POLLIN == 0 {
		t.Fatal("shim did not reach the held context lock")
	}
	buffer := make([]byte, unix.SizeofInotifyEvent+unix.NAME_MAX+1)
	if _, err := unix.Read(watch, buffer); err != nil {
		t.Fatal(err)
	}

	want := []byte(`{"ConfigVersion":4}`)
	writeCFConfig(t, ctx.CFHome, want)
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	lockHeld = false

	var got commandResult
	select {
	case got = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("shim did not finish after the context lock was released")
	}
	if got.code != exitOK {
		t.Fatalf("result = %#v", got)
	}
	actual, err := os.ReadFile(filepath.Join(ctx.CFHome, ".cf", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatalf("workspace configuration = %s, want %s", actual, want)
	}
	if strings.Contains(got.stderr, "seeded this workspace") {
		t.Fatalf("stderr = %q", got.stderr)
	}
}
