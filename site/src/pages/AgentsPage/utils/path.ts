export const getPathBasename = (path: string): string => {
	const slash = path.lastIndexOf("/");
	const basename = slash >= 0 ? path.substring(slash + 1) : path;
	return basename || path;
};

// Returns the parent directory of a path, or "" when the path has no directory
// component. Root-level paths (e.g. "/AGENTS.md") return "/".
export const getPathDirname = (path: string): string => {
	const slash = path.lastIndexOf("/");
	if (slash < 0) {
		return "";
	}
	if (slash === 0) {
		return "/";
	}
	return path.substring(0, slash);
};
