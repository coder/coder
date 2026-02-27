package codersdk

// WorkspaceAgentGitChangesResponse contains the git working changes
// for all repositories discovered in a workspace agent's directory.
type WorkspaceAgentGitChangesResponse struct {
	Repos []WorkspaceAgentRepoChanges `json:"repos"`
}

// WorkspaceAgentRepoChanges represents uncommitted working changes
// for a single git repository.
type WorkspaceAgentRepoChanges struct {
	// RepoRoot is the absolute path to the repository root.
	RepoRoot string `json:"repo_root"`
	// Branch is the current branch name, if available.
	Branch string `json:"branch,omitempty"`
	// RemoteOrigin is the URL of the "origin" remote, if configured.
	RemoteOrigin string `json:"remote_origin,omitempty"`
	// UnifiedDiff is the unified diff output of working changes.
	UnifiedDiff string `json:"unified_diff"`
	// UntrackedFiles lists files not tracked by git.
	UntrackedFiles []string `json:"untracked_files,omitempty"`
	// Additions is the total number of added lines.
	Additions int `json:"additions"`
	// Deletions is the total number of deleted lines.
	Deletions int `json:"deletions"`
	// ChangedFiles is the number of files with changes.
	ChangedFiles int `json:"changed_files"`
}
