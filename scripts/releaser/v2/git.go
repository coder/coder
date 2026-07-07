package v2

// gitOutput runs a read-only git command and returns trimmed stdout.
func gitOutput(exec CommandExecutor, args ...string) (string, error) {
	return exec.RunOutput("git", args...)
}

// gitRun runs a read-only git command, discarding stdout/stderr.
// Use this for commands where only the exit code matters (e.g.
// merge-base --is-ancestor).
func gitRun(exec CommandExecutor, args ...string) error {
	return exec.Run("git", args...)
}

// gitMutate runs a git command that modifies remote state (e.g.
// push, tag). In dry-run mode the command is printed instead of
// executed.
func gitMutate(exec CommandExecutor, args ...string) error {
	return exec.RunMutation("git", args...)
}
