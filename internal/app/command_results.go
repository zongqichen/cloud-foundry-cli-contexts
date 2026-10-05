package app

// Structured results for state-changing commands. Each command prints one of
// these as JSON when --json is set; otherwise it prints the human form. Errors
// are always reported on stderr with an exit code, never as JSON.

type createResult struct {
	Action    string `json:"action"`
	Context   string `json:"context"`
	Workspace string `json:"workspace"`
	CFHome    string `json:"cf_home"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
}

type removeResult struct {
	Action    string `json:"action"`
	Context   string `json:"context"`
	TrashPath string `json:"trash_path"`
	Warning   string `json:"credential_warning"`
}

type resetResult struct {
	Action    string `json:"action"`
	Workspace string `json:"workspace"`
	TrashPath string `json:"trash_path,omitempty"`
	Warning   string `json:"credential_warning,omitempty"`
}

type importResult struct {
	Action    string `json:"action"`
	Context   string `json:"context"`
	Workspace string `json:"workspace"`
	CFHome    string `json:"cf_home"`
}
