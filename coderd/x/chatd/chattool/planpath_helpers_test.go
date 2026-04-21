package chattool_test

func sharedPlanPathResolvedMessage(requestedPath, planPath string) string {
	return "the plan path " + requestedPath +
		" is no longer supported at the home root; use the chat-specific plan path: " + planPath
}

func planPathVerificationMessage(requestedPath string) string {
	return "the plan path " + requestedPath +
		" could not be verified because the workspace is currently unavailable to resolve the chat-specific plan path, try again shortly"
}

func editFilesBatchRejectedMessage(message string) string {
	return message + "; no files in this batch were applied"
}

func relativePlanPathMessage() string {
	return "plan files must use absolute paths; use the chat-specific absolute plan path"
}
