// The repository an origin names, such as "coder/coder" from
// "https://github.com/coder/coder.git". Falls back to the raw
// origin when the URL carries no owner/repo path.
export const originRepoLabel = (remoteOrigin: string | undefined): string => {
	if (!remoteOrigin) {
		return "";
	}

	let path: string;
	try {
		path = new URL(remoteOrigin).pathname;
	} catch {
		// Not a URL. An scp-style remote such as
		// "git@github.com:coder/coder.git" separates the host from
		// the path with a colon, so treat it like a slash.
		path = remoteOrigin.replaceAll(":", "/");
	}

	const segments = path
		.split("/")
		.filter(Boolean)
		.map((segment) =>
			segment.endsWith(".git") ? segment.slice(0, -4) : segment,
		);

	const owner = segments.at(-2);
	const repo = segments.at(-1);

	if (owner && repo) {
		return `${owner}/${repo}`;
	}

	return remoteOrigin;
};
