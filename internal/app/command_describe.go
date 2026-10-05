package app

import (
	"errors"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/config"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/envvar"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/store"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/workspace"
)

type describeOutput struct {
	Version     map[string]string `json:"version"`
	OfficialCF  string            `json:"official_cf"`
	ExitCodes   map[string]int    `json:"exit_codes"`
	Environment []envDescription  `json:"environment"`
	Workspace   describeWorkspace `json:"workspace"`
	Contexts    []contextListItem `json:"contexts"`
}

type describeWorkspace struct {
	Mode   string `json:"mode"`
	Root   string `json:"root,omitempty"`
	Source string `json:"source,omitempty"`
	CFHome string `json:"cf_home,omitempty"`
}

type envDescription struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// commandDescribe prints the full cfs contract in one read-only call: version,
// exit codes, environment variables, the current workspace resolution, and the
// contexts in that workspace. It never invokes cf, so it is fast, deterministic,
// and safe to run when not logged in.
func commandDescribe(options Options, args []string) int {
	flags := newFlagSet("describe", options.Stderr)
	jsonOutput := flags.Bool("json", false, "print JSON output")
	if code, ok := parseFlagSet(flags, args); !ok {
		return code
	}
	if flags.NArg() != 0 {
		fprintf(options.Stderr, "cfs: describe does not accept positional arguments\n")
		return exitUsage
	}

	cfg, err := config.Load()
	if err != nil {
		return reportConfigError(options, err)
	}

	out := describeOutput{
		Version: map[string]string{
			"version":    options.Version,
			"commit":     options.Commit,
			"build_date": options.BuildDate,
		},
		OfficialCF: cfg.RealCFPath,
		ExitCodes: map[string]int{
			"ok":          exitOK,
			"error":       exitError,
			"usage":       exitUsage,
			"unavailable": exitUnavailable,
			"busy":        exitTemporary,
		},
		Environment: environmentContract(),
		Contexts:    []contextListItem{},
	}

	workspaceDescription, items, code := describeWorkspaceState(options, cfg)
	if code != exitOK {
		return code
	}
	out.Workspace = workspaceDescription
	out.Contexts = items

	if *jsonOutput {
		return writeJSON(options, out)
	}
	printDescribe(options, out)
	return exitOK
}

// describeWorkspaceState resolves the workspace without touching cf. An external
// CF_HOME or an unresolved workspace is reported as such instead of failing.
func describeWorkspaceState(options Options, cfg config.Config) (describeWorkspace, []contextListItem, int) {
	if home, external := externalCFHome(); external {
		return describeWorkspace{Mode: "external", CFHome: home}, []contextListItem{}, exitOK
	}

	ws, err := workspace.Resolve(currentDirectory())
	if errors.Is(err, workspace.ErrNotFound) {
		return describeWorkspace{Mode: "unresolved"}, []contextListItem{}, exitOK
	}
	if err != nil {
		reportWorkspaceError(options, err)
		return describeWorkspace{}, nil, exitUnavailable
	}

	stateRoot, err := config.StateRoot(cfg)
	if err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return describeWorkspace{}, nil, exitError
	}
	stateStore := store.New(stateRoot)
	entries, err := stateStore.ListForWorkspace(ws)
	if err != nil {
		fprintf(options.Stderr, "cfs: list contexts: %v\n", err)
		return describeWorkspace{}, nil, exitError
	}
	items, err := contextItems(stateStore, ws, entries)
	if err != nil {
		fprintf(options.Stderr, "cfs: list contexts: %v\n", err)
		return describeWorkspace{}, nil, exitError
	}
	return describeWorkspace{Mode: "managed", Root: ws.Root, Source: ws.Source}, items, exitOK
}

func environmentContract() []envDescription {
	return []envDescription{
		{envvar.WorkspaceRoot, "Pin the workspace instead of discovering it from the directory"},
		{envvar.StateHome, "Override the directory that stores managed CF state"},
		{envvar.LockTimeout, "Maximum wait for a context lock (Go duration)"},
		{envvar.EphemeralTTL, "Idle time before gc reaps an ephemeral context (default 24h)"},
		{envvar.Disable, "Set to 1 to bypass workspace isolation for one invocation"},
		{envvar.NoUpdateCheck, "Set to 1 to disable interactive update notices"},
		{envvar.ConfigFile, "Override the path to the cfs configuration file"},
	}
}

func printDescribe(options Options, out describeOutput) {
	fprintf(options.Stdout, "cfs %s (commit %s, built %s)\n", out.Version["version"], out.Version["commit"], out.Version["build_date"])
	fprintf(options.Stdout, "CF CLI: %s\n", out.OfficialCF)
	switch out.Workspace.Mode {
	case "managed":
		fprintf(options.Stdout, "Workspace: %s (%s) [managed]\n", out.Workspace.Root, out.Workspace.Source)
	case "external":
		fprintf(options.Stdout, "Workspace: external (%s=%s)\n", envvar.CFHome, out.Workspace.CFHome)
	default:
		fprintf(options.Stdout, "Workspace: unresolved\n")
	}
	if len(out.Contexts) > 0 {
		fprintf(options.Stdout, "Contexts:\n")
		for _, item := range out.Contexts {
			marker := ""
			if item.Default {
				marker = " [default]"
			}
			fprintf(options.Stdout, "  %s (%s)%s\n", item.Name, item.Context, marker)
		}
	}
	fprintf(options.Stdout, "Run 'cfs describe --json' for the machine-readable contract.\n")
}
