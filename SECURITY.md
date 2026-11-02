# Security Policy

## Supported versions

Only the latest release receives security fixes. This project is pre-1.0, so
fixes are not routinely backported to earlier releases.

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use the
[private vulnerability reporting form](https://github.com/zongqichen/cloud-foundry-cli-contexts/security/advisories/new).

Include the affected version, operating system, reproduction steps, and the
expected security impact. Do not include live Cloud Foundry credentials, access
tokens, refresh tokens, client secrets, or production endpoint details.

## Security model

`cfs` delegates authentication and API communication to the official Cloud
Foundry CLI. It stores each workspace context's CF CLI home in a private local
state directory. It copies saved configuration only through the two modes below.

The project does not collect telemetry and must never log CF credentials or full
command lines. CF plugins are executable code and should be installed only from
trusted sources.

`cfs update` and the cached interactive update notice contact only the public
GitHub Releases API. Requests contain no Cloud Foundry target, credential,
workspace, context, or command data. The cache contains only public release
versions and timestamps. Set `CFS_NO_UPDATE_CHECK=1` to disable passive checks.
Update discovery never downloads or executes a replacement binary.

Managed `CF_HOME` directories contain the same authentication material as an
ordinary official CF CLI home. Owner-only filesystem permissions protect them
from other local users, but the files are not encrypted by `cfs`; backups and
host administrators may still access them.

An ordinary managed `cf` command automatically copies `$HOME/.cf/config.json`
when the default workspace configuration is absent at command start and the
source has a nonblank API `Target`. This copy requires no confirmation,
preserves the saved API and authentication configuration, and clears the
organization and space. The shim skips this copy if it finds an existing
workspace configuration at command start. Named invocations require explicit
import or login.

`cfs import` copies the global configuration into the default context or an
existing named context, including the saved organization and space. It requires
an interactive confirmation or `--yes`, and rejects a set `CF_HOME`. `--force`
permits replacement of an existing target but does not revoke its credentials.

Both modes reject symbolic links, invalid JSON, files larger than 4 MiB, and
source files accessible by other users. They write the destination atomically
with mode `0600` and never print the file contents. The source remains unchanged.
Each copy can contain access tokens, refresh tokens, and client secrets. These
independent snapshots do not synchronize token refreshes or later target
changes. Neither mode verifies live authentication. A saved API target can
contain expired credentials.

`cfs reset`, `cfs context remove`, and `cfs gc --apply` retain removed contexts
in recoverable trash indefinitely. Moving a context does not revoke its tokens.
Log out or revoke the credentials first when possible, and explicitly remove
the relevant trash entry under the configured cfs state directory when recovery
is no longer needed. Filesystem deletion may not constitute secure erasure on
snapshots or SSDs.

Unredacted `cfs status` and `cfs context status` invoke `cf target` and can print
operational metadata, including local paths, user identity, API endpoint,
organization, and space. They remove `CF_TRACE` from the probe environment and
never intentionally request tokens. Prefer their `--json --redact` form in
agent logs, issue reports, and other shared output.

The CI and release workflows select the latest Go 1.27 patch with
`check-latest: true`, so each run picks up available standard library security
fixes. Source installations require Go 1.26.10 or newer. Use the latest patch
of a supported Go release when building from source. The minimum-version CI job
reads its Go version from `go.mod`.
