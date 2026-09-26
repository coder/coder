package chattool_test

import "strconv"

func sharedPlanPathResolvedMessage(requestedPath, planPath string) string {
	return "the plan path " + requestedPath +
		" is no longer supported at the home root; use the chat-specific plan path: " + planPath
}

func planPathVerificationMessage(requestedPath string) string {
	return "the plan path " + requestedPath +
		" could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly"
}

// editFilesFileRejectedMessage is the edit_files error result when its
// only file, edited by edits[index], was rejected with message.
func editFilesFileRejectedMessage(path string, index int, message string) string {
	return "No files were applied.\n- " + path + " (edits[" + strconv.Itoa(index) + "]): " + message
}

func relativePlanPathMessage() string {
	return "plan files must use absolute paths; use the chat-specific absolute plan path"
}
