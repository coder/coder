export const getPathBasename = (path: string): string => {
	const slash = path.lastIndexOf("/");
	return slash >= 0 ? path.substring(slash + 1) : path;
};
