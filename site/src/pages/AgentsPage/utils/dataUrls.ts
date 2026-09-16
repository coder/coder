type DecodedDataURL = {
	mediaType: string;
	isBase64: boolean;
	bytes: Uint8Array<ArrayBuffer>;
};

// Data URLs are decoded by hand because the production CSP excludes data:
// from connect-src (so fetch cannot read them) and the Safari 16 support
// baseline predates Uint8Array.fromBase64.
export const decodeDataURL = (url: string): DecodedDataURL | null => {
	const commaIndex = url.indexOf(",");
	const scheme = url.slice(0, "data:".length).toLowerCase();
	if (scheme !== "data:" || commaIndex === -1) {
		return null;
	}
	const params = url.slice("data:".length, commaIndex).split(";");
	const payload = url.slice(commaIndex + 1);
	const isBase64 = params.at(-1)?.toLowerCase() === "base64";
	try {
		const bytes = isBase64
			? Uint8Array.from(atob(payload), (char) => char.charCodeAt(0))
			: new TextEncoder().encode(decodeURIComponent(payload));
		return {
			mediaType: params[0].trim(),
			isBase64,
			bytes,
		};
	} catch {
		return null;
	}
};
