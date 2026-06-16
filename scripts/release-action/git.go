package main

// gitOutput runs a read-only git command and returns trimmed stdout.
func gitOutput(exec CommandExecutor, args ...string) (string, error) {
	return exec.RunOutput("git", args...)
}

// gitRun runs a git command, discarding stdout/stderr. Use this
// for commands where only the exit code matters (e.g. merge-base
// --is-ancestor).
func gitRun(exec CommandExecutor, args ...string) error {
	return exec.Run("git", args...)
}
