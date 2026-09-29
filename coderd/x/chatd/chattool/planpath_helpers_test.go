package chattool_test

func sharedPlanPathResolvedMessage(requestedPath, planPath string) string {
	return "the plan path " + requestedPath +
		" is no longer supported at the home root; use the chat-specific plan path: " + planPath
}

func planPathVerificationMessage(requestedPath string) string {
	return "the plan path " + requestedPath +
		" could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly"
}

// editFilesOnlyEditRejectedMessage is the edit_files error result for
// a call whose only edit, to path, was rejected with message.
func editFilesOnlyEditRejectedMessage(path, message string) string {
	return "Applied 0 of 1 edits. Not applied:\n- edits[0] (" + path + "): " + message +
		". " + path + " is unchanged; fix and resend only these edits."
}

func relativePlanPathMessage() string {
	return "plan files must use absolute paths; use the chat-specific absolute plan path"
}
