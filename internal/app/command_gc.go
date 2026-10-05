package app

import (
	"fmt"
	"os"
	"time"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/config"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/envvar"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/lock"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
)

const defaultEphemeralTTL = 24 * time.Hour

type gcAction string

const (
	gcWouldTrash gcAction = "would-trash"
	gcBusy       gcAction = "busy"
	gcError      gcAction = "error"
	gcTrashed    gcAction = "trashed"
)

type gcEntry struct {
	Context   string   `json:"context"`
	Name      string   `json:"name"`
	Workspace string   `json:"workspace"`
	Action    gcAction `json:"action"`
	Reason    string   `json:"reason,omitempty"`
	Path      string   `json:"path,omitempty"`
}

func commandGC(options Options, args []string) int {
	flags := newFlagSet("gc", options.Stderr)
	apply := flags.Bool("apply", false, "move orphaned contexts to trash")
	jsonOutput := flags.Bool("json", false, "print JSON output")
	if code, ok := parseFlagSet(flags, args); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: gc does not accept positional arguments\n")
		return exitUsage
	}

	cfg, err := config.Load()
	if err != nil {
		return reportConfigError(options, err)
	}
	stateRoot, err := config.StateRoot(cfg)
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitError
	}
	stateStore := store.New(stateRoot)
	entries, err := stateStore.List()
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitError
	}

	ttl, err := ephemeralTTL()
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}

	results, failed := collectGarbage(entries, stateStore, *apply, time.Now().UTC(), ttl)
	if *jsonOutput {
		output := map[string]any{"contexts": results, "applied": *apply}
		if containsGCAction(results, gcTrashed) {
			output["warning"] = trashCredentialWarning
		}
		if code := writeJSON(options, output); code != exitOK {
			return code
		}
	} else {
		printGCResults(options, results, *apply)
	}
	if failed {
		return exitError
	}
	return exitOK
}

func collectGarbage(entries []store.Entry, stateStore store.Store, apply bool, now time.Time, ttl time.Duration) ([]gcEntry, bool) {
	results := []gcEntry{}
	failed := false
	for _, entry := range entries {
		reason := reapReason(entry, now, ttl)
		if reason == "" {
			continue
		}
		result := gcEntry{
			Context:   shortID(entry.Context.ID),
			Name:      entry.Metadata.ContextName,
			Workspace: entry.Metadata.Workspace,
			Action:    gcWouldTrash,
			Reason:    reason,
		}
		if apply {
			workspaceLock, lockErr := lock.Acquire(entry.Context.LockPath, 0)
			if lockErr != nil {
				result.Action = gcBusy
				failed = true
			} else {
				destination, moveErr := stateStore.MoveToTrash(entry.Context)
				_ = workspaceLock.Release()
				if moveErr != nil {
					result.Action = gcError
					failed = true
				} else {
					result.Action = gcTrashed
					result.Path = destination
				}
			}
		}
		results = append(results, result)
	}
	return results, failed
}

// reapReason reports why an entry is eligible for gc, or "" if it is not. A
// context is reaped when its workspace no longer exists, or when it is
// ephemeral and has been idle longer than the TTL.
func reapReason(entry store.Entry, now time.Time, ttl time.Duration) string {
	if entry.Orphaned {
		return "orphaned"
	}
	if entry.Metadata.Ephemeral && now.Sub(entry.Metadata.LastUsedAt) >= ttl {
		return "ephemeral-expired"
	}
	return ""
}

func ephemeralTTL() (time.Duration, error) {
	value := os.Getenv(envvar.EphemeralTTL)
	if value == "" {
		return defaultEphemeralTTL, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil || ttl < 0 {
		return 0, fmt.Errorf("invalid %s %q", envvar.EphemeralTTL, value)
	}
	return ttl, nil
}

func printGCResults(options Options, results []gcEntry, applied bool) {
	if len(results) == 0 {
		fprintf(options.Stdout, "No orphaned workspace contexts found.\n")
		return
	}
	for _, result := range results {
		fprintf(options.Stdout, "%-12s %s [%s:%s]\n", result.Action, result.Workspace, result.Name, result.Context)
	}
	if !applied {
		fprintf(options.Stdout, "Run 'cfs gc --apply' to move these contexts to trash.\n")
	} else if containsGCAction(results, gcTrashed) {
		fprintf(options.Stdout, "%s\n", trashCredentialWarning)
	}
}

func containsGCAction(results []gcEntry, action gcAction) bool {
	for _, result := range results {
		if result.Action == action {
			return true
		}
	}
	return false
}
