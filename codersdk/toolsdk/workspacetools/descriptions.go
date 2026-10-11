package workspacetools

// Model-facing descriptions shared by every surface that exposes the
// workspace tools. Coder Agents declares argument descriptions in struct
// tags, which must be string literals, so parity tests compare those
// tags against the constants below.

const (
	// ExecuteCommandDescription describes the execute tool's command.
	ExecuteCommandDescription = `The shell command to execute. Runs under "sh -c" (POSIX).`
	// ExecuteTimeoutDescription describes the execute tool's timeout.
	ExecuteTimeoutDescription = "How long to wait for completion (e.g. '30s', '5m'). Default is 10s. The process keeps running if this expires and you get a background_process_id to re-attach. Only applies to foreground commands."
	// ExecuteWorkDirDescription describes the execute tool's workdir.
	ExecuteWorkDirDescription = "Working directory for the command."
	// ProcessOutputWaitTimeoutDescription describes the process output
	// tool's wait_timeout.
	ProcessOutputWaitTimeoutDescription = "Override the default 10s block duration. The call blocks until the process exits or this timeout is reached. Set to '0s' for an immediate snapshot without waiting."
	// EditFilesFilesDescription describes the edit files tool's files.
	EditFilesFilesDescription = "Files to edit. Every entry must include path and at least one edit."
	// WriteFileContentDescription describes the write file tool's content.
	WriteFileContentDescription = "Complete file contents. Replaces any existing contents."
	// ProcessListDescription describes the process list tool.
	ProcessListDescription = "List all tracked processes in the workspace. " +
		"Returns process IDs, commands, status (running or " +
		"exited), and exit codes. Use this to discover " +
		"processes or check which are still running."
	// ReadFileDescription describes the read file tool.
	ReadFileDescription = "Read a file from the workspace. Returns line-numbered content. " +
		"The offset parameter is a 1-based line number (default: 1). " +
		"The limit parameter is the number of lines to return (default: 2000). " +
		"For large files, use offset and limit to paginate."
	// EditFilesDescription describes the edit files tool.
	EditFilesDescription = "Perform edits on one or more files by replacing old_text with" +
		" new_text. Each entry in files must include the absolute path" +
		" of the file to edit and at least one edit. Matching is fuzzy" +
		" (tolerates whitespace and indentation differences) and preserves" +
		" the file's existing indentation and line endings. Errors if" +
		" old_text matches zero locations, or more than one unless" +
		" replace_all is set. All edits in a batch are validated before" +
		" any file is written."
)

// ExecuteRunInBackgroundDescription describes the execute tool's
// run_in_background argument.
func ExecuteRunInBackgroundDescription(names ToolNames) string {
	return "Run without blocking. Use for persistent processes (dev servers, file watchers) or when you want to continue working while a command runs and check the result later with " + names.ProcessOutput + ". For commands whose result you need before continuing, prefer foreground with a longer timeout. Use this parameter instead of shell '&', which leaves an untracked process that " + names.ProcessOutput + " cannot read."
}

// ExecuteDescription describes the execute tool.
func ExecuteDescription(names ToolNames) string {
	return "Execute a shell command in the workspace. Runs under \"sh -c\" (POSIX). Waits for completion up to the timeout (default 10s, override with the timeout parameter e.g. '30s', '5m'). If the command exceeds the timeout, the response includes a background_process_id; use " + names.ProcessOutput + " with that ID to re-attach and wait for the result. Use run_in_background=true for persistent processes (dev servers, file watchers) or when you want to continue other work while the command runs. Never use shell '&' for backgrounding."
}

// ProcessOutputDescription describes the process output tool.
func ProcessOutputDescription(names ToolNames) string {
	return "Retrieve output from a tracked process by ID. " +
		"Use the process_id returned by " + names.Execute + " with " +
		"run_in_background=true or from a timed-out " +
		names.Execute + "'s background_process_id. Blocks up to " +
		"10s for the process to exit, then returns the " +
		"output and exit_code. If still running after " +
		"the timeout, returns the output so far. Use " +
		"wait_timeout to override the default 10s wait " +
		"(e.g. '30s', or '0s' for an immediate snapshot " +
		"without waiting)."
}

// ProcessSignalDescription describes the process signal tool.
func ProcessSignalDescription(names ToolNames) string {
	return "Send a signal to a tracked process. " +
		"Use \"terminate\" (SIGTERM) for graceful shutdown " +
		"or \"kill\" (SIGKILL) to force stop. Use the " +
		"process_id returned by " + names.Execute + " with " +
		"run_in_background=true or from " + names.ProcessList + "."
}
