/**
 * Returns `url` parsed against `base`, or undefined when it is not a valid
 * URL. Use it instead of `URL.parse`, which some browsers in the
 * browserslist in package.json do not have.
 */
export const parseURL = (url: string, base?: string): URL | undefined => {
	try {
		return new URL(url, base);
	} catch {
		return undefined;
	}
};

export const truncateURI = (uri: string) => {
	if (uri.startsWith("file://")) {
		const path = uri.slice(7);
		// Slightly shorter truncation for this context if needed
		if (path.length > 35) {
			const start = path.slice(0, 15);
			const end = path.slice(-15);
			return `${start}...${end}`;
		}
		return path;
	}

	try {
		const url = new URL(uri);
		const fullUrl = url.toString();
		// Slightly shorter truncation
		if (fullUrl.length > 30) {
			const start = fullUrl.slice(0, 15);
			const end = fullUrl.slice(-15);
			return `${start}...${end}`;
		}
		return fullUrl;
	} catch {
		// Slightly shorter truncation
		if (uri.length > 20) {
			const start = uri.slice(0, 10);
			const end = uri.slice(-10);
			return `${start}...${end}`;
		}
		return uri;
	}
};
