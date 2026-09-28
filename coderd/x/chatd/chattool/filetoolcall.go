package chattool

import (
	"errors"
	"fmt"
)

// Names of the file tools that send tool call headers.
const (
	EditFilesToolName = "edit_files"
	WriteFileToolName = "write_file"
)

// fileToolWords are the words edit_files and write_file use in tool call
// results. change names what the tool does to the file: "edit" or
// "write".
func fileToolWords(change string, id ToolCallIdentity) ToolCallWords {
	return ToolCallWords{
		NotRun:       change + " not applied",
		Effect:       "the " + change + " may have been applied",
		Check:        fmt.Sprintf("Check the file before changing it again (tool call %s).", id.UUID()),
		UnknownCheck: "Check the file before changing it again.",
	}
}

// fileConnErrorText is ConnErrorText for edit_files and write_file. A
// stopped workspace has no running agent but keeps its disk, so an
// earlier attempt's change may be there: only a chat without a workspace
// and a deleted workspace keep the tool's usual error.
func fileConnErrorText(err error, words ToolCallWords) (text string, ok bool) {
	if errors.Is(err, ErrChatHasNoWorkspace) || errors.Is(err, ErrWorkspaceDeleted) {
		return "", false
	}
	return UnknownOutcome(fmt.Sprintf("the workspace agent could not be reached (%v)", err), words.Effect, words.Check), true
}
