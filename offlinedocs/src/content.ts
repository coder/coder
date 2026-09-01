export type FilePath = string;
export type UrlPath = string;

export type Route = {
	path: FilePath;
	title: string;
	description?: string;
	children?: Route[];
};

export type Manifest = { versions: string[]; routes: Route[] };

export type NavItem = { title: string; path: UrlPath; children?: NavItem[] };

export type Nav = NavItem[];

export type DocsPageData = {
	content: string;
	route: Route;
	title: string;
};

const removeTrailingSlash = (filePath: string) => filePath.replace(/\/+$/, "");

const removeMkdExtension = (filePath: string) => filePath.replace(/\.md/g, "");

const removeIndexFilename = (filePath: string) => {
	if (filePath.endsWith("index")) {
		return filePath.replace("index", "");
	}

	return filePath;
};

const removeREADMEName = (filePath: string) => {
	if (filePath.startsWith("README")) {
		return filePath.replace("README", "");
	}

	return filePath;
};

// transformLinkUri converts the links in the markdown file to
// href html links. All index page routes are the directory name, and all
// other routes are the filename without the .md extension.
// This means all relative links are off by one directory on non-index pages.
//
// index.md -> ./subdir/file = ./subdir/file
// index.md -> ../file-next-to-index = ./file-next-to-index
// file.md -> ./subdir/file = ../subdir/file
// file.md -> ../file-next-to-file = ../file-next-to-file
export const transformLinkUriSource = (sourceFile: string) => {
	return (href = "") => {
		const isExternal = href.startsWith("http") || href.startsWith("https");
		if (!isExternal) {
			href = removeMkdExtension(href);

			const sourceWithoutMd = removeMkdExtension(sourceFile);
			if (!sourceWithoutMd.endsWith("index")) {
				href = `../${href}`;
			}

			href = removeIndexFilename(href);
			href = removeREADMEName(href);
		}
		return href;
	};
};

export const transformFilePathToUrlPath = (filePath: string) => {
	let urlPath = removeMkdExtension(filePath);

	if (urlPath.startsWith("./")) {
		urlPath = urlPath.replace("./", "");
	}

	urlPath = removeIndexFilename(urlPath);
	urlPath = removeREADMEName(urlPath);

	if (urlPath.endsWith("/")) {
		urlPath = removeTrailingSlash(urlPath);
	}

	return urlPath;
};

export const mapRoutes = (manifest: Manifest): Record<UrlPath, Route> => {
	const paths: Record<UrlPath, Route> = {};

	const addPaths = (routes: Route[]) => {
		for (const route of routes) {
			paths[transformFilePathToUrlPath(route.path)] = route;

			if (route.children) {
				addPaths(route.children);
			}
		}
	};

	addPaths(manifest.routes);

	return paths;
};

export const getNavigation = (manifest: Manifest): Nav => {
	const getNavItem = (route: Route, parentPath?: UrlPath): NavItem => {
		const urlPath = parentPath
			? `${parentPath}/${transformFilePathToUrlPath(route.path)}`
			: transformFilePathToUrlPath(route.path);
		const navItem: NavItem = {
			title: route.title,
			path: urlPath,
		};

		if (route.children) {
			navItem.children = [];

			for (const childRoute of route.children) {
				navItem.children.push(getNavItem(childRoute));
			}
		}

		return navItem;
	};

	const navigation: Nav = [];

	for (const route of manifest.routes) {
		navigation.push(getNavItem(route));
	}

	return navigation;
};

export const urlPathFromLocation = (pathname: string) => {
	let urlPath = pathname;
	if (urlPath.startsWith("/")) {
		urlPath = urlPath.slice(1);
	}
	if (urlPath.endsWith("/")) {
		urlPath = urlPath.slice(0, -1);
	}
	return urlPath;
};

export const hrefForPath = (urlPath: string) =>
	urlPath === "" ? "/" : `/${urlPath}/`;
