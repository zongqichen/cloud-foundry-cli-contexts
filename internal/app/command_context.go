package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/cfhome"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/config"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/contextname"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/lock"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/workspace"
)

type contextListItem struct {
	Name    string          `json:"name"`
	Context string          `json:"context"`
	Default bool            `json:"default"`
	Created bool            `json:"created"`
	Target  *cfhome.Summary `json:"target,omitempty"`
}

func commandContext(options Options, args []string) int {
	if len(args) == 0 || isHelpRequest(args) {
		printContextHelp(options.Stdout)
		return exitOK
	}
	if args[0] == "help" {
		if len(args) == 1 {
			printContextHelp(options.Stdout)
			return exitOK
		}
		args = append([]string{args[1], "--help"}, args[2:]...)
	}

	switch args[0] {
	case "create":
		return commandContextCreate(options, args[1:])
	case "list", "ls":
		return commandContextList(options, args[1:])
	case "status":
		return commandContextStatus(options, args[1:])
	case "remove", "rm":
		return commandContextRemove(options, args[1:])
	default:
		fprintf(options.Stderr, "cfs: unknown context command %q\n", args[0])
		fprintf(options.Stderr, "Run 'cfs help context' for usage.\n")
		return exitUsage
	}
}

func commandContextCreate(options Options, args []string) (exitCode int) {
	if isHelpRequest(args) {
		printContextSubcommandHelp(options.Stdout, "Create a named or ephemeral context.", "cfs context create [<name>] [--ephemeral] [--json]")
		return exitOK
	}
	var name string
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
		rest = args[1:]
	}
	flags := newContextFlagSet("create", options.Stderr)
	jsonOutput := flags.Bool("json", false, "print JSON output")
	ephemeral := flags.Bool("ephemeral", false, "create a uniquely named context that gc reaps once idle")
	if code, ok := parseFlagSet(flags, rest); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: context create does not accept extra arguments\n")
		return exitUsage
	}
	if name == "" {
		if !*ephemeral {
			fprintf(options.Stderr, "cfs: context create requires a name (or --ephemeral)\n")
			return exitUsage
		}
		generated, err := generateEphemeralName()
		if err != nil {
			fprintf(options.Stderr, "cfs: %v\n", err)
			return exitError
		}
		name = generated
	}
	if err := contextname.Validate(name); err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}
	managed, code, ok := managedContextForCommand(options, name)
	if !ok {
		return code
	}
	timeout, err := lockTimeout()
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}
	if err := managed.Store.PrepareRoot(); err != nil {
		fprintf(options.Stderr, "cfs: prepare context store: %v\n", err)
		return exitError
	}
	contextLock, err := lock.Acquire(managed.Context.LockPath, timeout)
	if errors.Is(err, lock.ErrBusy) {
		fprintf(options.Stderr, "cfs: context %q already has an active operation\n", name)
		return exitTemporary
	}
	if err != nil {
		fprintf(options.Stderr, "cfs: create context %q: %v\n", name, err)
		return exitError
	}
	defer func() {
		if releaseErr := contextLock.Release(); releaseErr != nil {
			fprintf(options.Stderr, "cfs: %v\n", releaseErr)
			if exitCode == exitOK {
				exitCode = exitError
			}
		}
	}()
	if _, err := managed.Store.ValidateForWorkspace(managed.Context, managed.Workspace); err == nil {
		fprintf(options.Stderr, "cfs: context %q already exists\n", name)
		return exitUsage
	} else if !errors.Is(err, os.ErrNotExist) {
		fprintf(options.Stderr, "cfs: inspect context %q: %v\n", name, err)
		return exitError
	}
	ensure := managed.Store.Ensure
	if *ephemeral {
		ensure = managed.Store.EnsureEphemeral
	}
	if err := ensure(managed.Context, managed.Workspace); err != nil {
		fprintf(options.Stderr, "cfs: create context %q: %v\n", name, err)
		return exitError
	}
	if *jsonOutput {
		return writeJSON(options, createResult{
			Action:    "created",
			Context:   name,
			Workspace: managed.Workspace.Root,
			CFHome:    managed.Context.CFHome,
			Ephemeral: *ephemeral,
		})
	}
	if *ephemeral {
		fprintf(options.Stdout, "Created ephemeral context %q.\n", name)
	} else {
		fprintf(options.Stdout, "Created context %q.\n", name)
	}
	return exitOK
}

// generateEphemeralName returns a unique, validation-safe context name for an
// ephemeral context, e.g. "eph-1a2b3c4d".
func generateEphemeralName() (string, error) {
	raw := make([]byte, 4)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate ephemeral context name: %w", err)
	}
	return "eph-" + hex.EncodeToString(raw), nil
}

func commandContextList(options Options, args []string) int {
	flags := newContextFlagSet("list", options.Stderr)
	jsonOutput := flags.Bool("json", false, "print JSON output")
	targets := flags.Bool("targets", false, "include each context's CF target (api/org/space)")
	if code, ok := parseFlagSet(flags, args); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: context list does not accept positional arguments\n")
		return exitUsage
	}
	cfg, err := config.Load()
	if err != nil {
		return reportConfigError(options, err)
	}
	ws, err := workspace.Resolve(currentDirectory())
	if err != nil {
		reportWorkspaceError(options, err)
		return exitUnavailable
	}
	stateRoot, err := config.StateRoot(cfg)
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitError
	}
	stateStore := store.New(stateRoot)
	entries, err := stateStore.ListForWorkspace(ws)
	if err != nil {
		fprintf(options.Stderr, "cfs: list contexts: %v\n", err)
		return exitError
	}
	items, err := contextItems(stateStore, ws, entries)
	if err != nil {
		fprintf(options.Stderr, "cfs: list contexts: %v\n", err)
		return exitError
	}
	if *targets {
		if err := attachContextTargets(items, entries); err != nil {
			fprintf(options.Stderr, "cfs: inspect context targets: %v\n", err)
			return exitError
		}
	}
	if *jsonOutput {
		return writeJSON(options, map[string]any{"contexts": items})
	}
	printContextList(options.Stdout, items, *targets)
	return exitOK
}

// attachContextTargets fills in each created context's target summary by reading
// its stored CF configuration directly. It never invokes cf and never reads
// credentials beyond the api/org/space names.
func attachContextTargets(items []contextListItem, entries []store.Entry) error {
	homes := make(map[string]string, len(entries))
	for _, entry := range entries {
		homes[entry.Metadata.ContextName] = entry.Context.CFHome
	}
	for index := range items {
		home, ok := homes[items[index].Name]
		if !ok {
			continue
		}
		summary, present, err := cfhome.Summarize(home)
		if err != nil {
			return err
		}
		if present {
			items[index].Target = &summary
		}
	}
	return nil
}

func commandContextStatus(options Options, args []string) int {
	if len(args) == 0 {
		fprintf(options.Stderr, "cfs: context status requires a name\n")
		return exitUsage
	}
	if isHelpRequest(args) {
		printContextSubcommandHelp(options.Stdout, "Show a named context and its CF target.", "cfs context status <name> [--json] [--redact]")
		return exitOK
	}
	name := args[0]
	if err := contextname.Validate(name); err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}
	flags := newContextFlagSet("status", options.Stderr)
	jsonOutput := flags.Bool("json", false, "print JSON output")
	redact := flags.Bool("redact", false, "omit paths and CF target details")
	if code, ok := parseFlagSet(flags, args[1:]); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: context status does not accept extra arguments\n")
		return exitUsage
	}
	return reportStatus(options, name, true, *jsonOutput, *redact)
}

func commandContextRemove(options Options, args []string) int {
	if len(args) == 0 {
		fprintf(options.Stderr, "cfs: context remove requires a name\n")
		return exitUsage
	}
	if isHelpRequest(args) {
		printContextSubcommandHelp(options.Stdout, "Move a named context to recoverable trash.", "cfs context remove <name> [--yes] [--json]")
		return exitOK
	}
	name := args[0]
	if err := contextname.Validate(name); err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}
	flags := newContextFlagSet("remove", options.Stderr)
	yes := flags.Bool("yes", false, "confirm removal without prompting")
	jsonOutput := flags.Bool("json", false, "print JSON output")
	if code, ok := parseFlagSet(flags, args[1:]); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: context remove does not accept extra arguments\n")
		return exitUsage
	}
	managed, code, ok := managedContextForCommand(options, name)
	if !ok {
		return code
	}
	if _, err := managed.Store.ValidateForWorkspace(managed.Context, managed.Workspace); errors.Is(err, os.ErrNotExist) {
		fprintf(options.Stderr, "cfs: context %q does not exist\n", name)
		return exitUnavailable
	} else if err != nil {
		fprintf(options.Stderr, "cfs: inspect context %q: %v\n", name, err)
		return exitError
	}
	if !*yes {
		confirmed, code := confirm(options, "context remove", "Remove context "+name+" from "+managed.Workspace.Root+"?")
		if !confirmed {
			return code
		}
	}
	timeout, err := lockTimeout()
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUsage
	}
	contextLock, err := lock.Acquire(managed.Context.LockPath, timeout)
	if errors.Is(err, lock.ErrBusy) {
		fprintf(options.Stderr, "cfs: context %q already has an active CF command\n", name)
		return exitTemporary
	}
	if err != nil {
		fprintf(options.Stderr, "cfs: remove context %q: %v\n", name, err)
		return exitError
	}
	if _, validateErr := managed.Store.ValidateForWorkspace(managed.Context, managed.Workspace); validateErr != nil {
		combinedErr := errors.Join(validateErr, contextLock.Release())
		if errors.Is(validateErr, os.ErrNotExist) {
			fprintf(options.Stderr, "cfs: context %q does not exist\n", name)
			return exitUnavailable
		}
		fprintf(options.Stderr, "cfs: inspect context %q: %v\n", name, combinedErr)
		return exitError
	}
	destination, moveErr := managed.Store.MoveToTrash(managed.Context)
	releaseErr := contextLock.Release()
	if moveErr != nil {
		fprintf(options.Stderr, "cfs: remove context %q: %v\n", name, moveErr)
		return exitError
	}
	if releaseErr != nil {
		fprintf(options.Stderr, "cfs: %v\n", releaseErr)
		return exitError
	}
	if *jsonOutput {
		return writeJSON(options, removeResult{
			Action:    "removed",
			Context:   name,
			TrashPath: destination,
			Warning:   trashCredentialWarning,
		})
	}
	fprintf(options.Stdout, "Moved context %q to %s\n", name, destination)
	fprintf(options.Stdout, "%s\n", trashCredentialWarning)
	return exitOK
}

func managedContextForCommand(options Options, name string) (managedContext, int, bool) {
	cfg, err := config.Load()
	if err != nil {
		return managedContext{}, reportConfigError(options, err), false
	}
	managed, err := resolveManagedContextForName(cfg, name)
	if err != nil {
		reportWorkspaceError(options, err)
		return managedContext{}, exitUnavailable, false
	}
	return managed, exitOK, true
}

func contextItems(stateStore store.Store, ws workspace.Workspace, entries []store.Entry) ([]contextListItem, error) {
	items := make([]contextListItem, 0, len(entries)+1)
	foundDefault := false
	for _, entry := range entries {
		name := entry.Metadata.ContextName
		items = append(items, contextListItem{
			Name: name, Context: shortID(entry.Context.ID), Default: name == contextname.Default, Created: true,
		})
		foundDefault = foundDefault || name == contextname.Default
	}
	if !foundDefault {
		ctx, err := stateStore.ContextFor(ws)
		if err != nil {
			return nil, err
		}
		items = append(items, contextListItem{
			Name: contextname.Default, Context: shortID(ctx.ID), Default: true, Created: false,
		})
	}
	sort.Slice(items, func(first, second int) bool {
		if items[first].Default != items[second].Default {
			return items[first].Default
		}
		return items[first].Name < items[second].Name
	})
	return items, nil
}

func printContextList(output io.Writer, items []contextListItem, showTargets bool) {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if showTargets {
		fprintf(writer, "NAME\tSTATE\tCONTEXT\tTARGET\n")
	} else {
		fprintf(writer, "NAME\tSTATE\tCONTEXT\n")
	}
	for _, item := range items {
		state := "created"
		if !item.Created {
			state = "implicit"
		}
		if showTargets {
			fprintf(writer, "%s\t%s\t%s\t%s\n", item.Name, state, item.Context, targetColumn(item.Target))
		} else {
			fprintf(writer, "%s\t%s\t%s\n", item.Name, state, item.Context)
		}
	}
	_ = writer.Flush()
}

func targetColumn(summary *cfhome.Summary) string {
	if summary == nil {
		return "-"
	}
	if summary.Org == "" && summary.Space == "" {
		return summary.API
	}
	return fmt.Sprintf("%s (%s/%s)", summary.API, summary.Org, summary.Space)
}

func newContextFlagSet(name string, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet("cfs context "+name, flag.ContinueOnError)
	flags.SetOutput(output)
	return flags
}

func printContextHelp(output io.Writer) {
	fprintf(output, "Manage named Cloud Foundry contexts in the current workspace.\n\n")
	fprintf(output, "Usage:\n  cfs context <command> [options]\n\nCommands:\n")
	fprintf(output, "  create [<name>]        Create a context (--ephemeral self-names and gc-reaps)\n")
	fprintf(output, "  list                   List contexts (--targets adds api/org/space)\n")
	fprintf(output, "  status <name>          Show a context and its CF target\n")
	fprintf(output, "  remove <name>          Move a context to recoverable trash\n")
	fprintf(output, "\nRun CF commands with a named context:\n  cfs -c <name> <cf arguments...>\n")
	fprintf(output, "\nNames use 1-63 lowercase letters or digits; '.', '-', and '_' are allowed internally.\n")
}

func printContextSubcommandHelp(output io.Writer, summary, usage string) {
	fprintf(output, "%s\n\nUsage:\n  %s\n", summary, usage)
}
