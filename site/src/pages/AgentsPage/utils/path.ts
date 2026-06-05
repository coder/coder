export const getPathBasename = (path: string): string => {
	const slash = path.lastIndexOf("/");
	const basename = slash >= 0 ? path.substring(slash + 1) : path;
	return basename || path;
};
