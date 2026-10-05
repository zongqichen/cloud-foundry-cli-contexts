package app

import (
	"io"
	"os"

	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/cfhome"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/config"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/envvar"
	"github.com/zongqichen/cloud-foundry-cli-contexts/internal/runner"
)

// commandGlobal runs an official CF command against the global CF target,
// deliberately bypassing workspace isolation. It is the discoverable, one-shot
// equivalent of setting CFS_DISABLE=1 for a single invocation, giving humans an
// escape hatch out of the wrong workspace and giving agents an explicit way to
// address the global target without depending on the current directory.
func commandGlobal(options Options, args []string) int {
	if isHelpRequest(args) {
		printGlobalHelp(options.Stdout)
		return exitOK
	}
	if len(args) == 0 {
		fprintf(options.Stderr, "cfs: global requires a CF command\n")
		fprintf(options.Stderr, "Usage:\n  cfs global <cf arguments...>\n")
		return exitUsage
	}

	cfg, err := config.Load()
	if err != nil {
		return reportConfigError(options, err)
	}
	if err := validateRealCF(cfg.RealCFPath); err != nil {
		fprintf(options.Stderr, "cfs: %v\n", err)
		return exitUnavailable
	}

	// Guard rail: announce the target so a stray `cf delete` or `cf login`
	// cannot silently hit the global state while the caller believes they are
	// still isolated. The notice goes to stderr so it never pollutes stdout
	// that an agent may be parsing.
	fprintf(options.Stderr, "cfs: running cf against the global CF home %s\n", globalCFHome())

	// Strip the managed marker so the child never looks like it is running
	// inside a cfs-managed context. CF_HOME is left untouched: if the caller
	// set one explicitly, that is their global target.
	env := runner.WithoutEnv(os.Environ(), envvar.ActiveContext)
	return invokeOfficial(options, cfg.RealCFPath, args, env)
}

func globalCFHome() string {
	if home := os.Getenv(envvar.CFHome); home != "" {
		return home
	}
	if home, err := cfhome.Default(); err == nil {
		return home
	}
	return "its default location"
}

func printGlobalHelp(output io.Writer) {
	summary := "Run a CF command against the global target, bypassing isolation"
	if command, ok := findCommand("global"); ok {
		summary = command.summary
	}
	fprintf(output, "%s.\n\n", summary)
	fprintf(output, "Usage:\n  cfs global <cf arguments...>\n\n")
	fprintf(output, "Use this to escape the current workspace for a single command, for example\n")
	fprintf(output, "after entering the wrong workspace or to log in somewhere else.\n\n")
	fprintf(output, "Examples:\n  cfs global login --sso\n  cfs global target\n  cfs global apps\n")
}
