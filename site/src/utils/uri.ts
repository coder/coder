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
