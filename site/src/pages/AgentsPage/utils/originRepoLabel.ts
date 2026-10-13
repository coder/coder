// The repository path an origin names, such as "coder/coder" from
// "https://github.com/coder/coder.git".
export const originRepoLabel = (origin: string): string => {
	let path: string;
	try {
		path = new URL(origin).pathname;
	} catch {
		// An scp-style remote such as "git@github.com:coder/coder.git"
		// is not a URL. Its path follows the colon.
		path = origin.slice(origin.indexOf(":") + 1);
	}

	// A URL path starts with a slash, and an origin can end with one.
	// Dropping empty segments removes both from the label.
	let repoPath = path.split("/").filter(Boolean).join("/");

	// Clone URLs often end in ".git", but the repository name does not
	// include it.
	if (repoPath.endsWith(".git")) {
		repoPath = repoPath.slice(0, -".git".length);
	}

	// A host-only origin has no path, so show the origin itself.
	return repoPath || origin;
};
