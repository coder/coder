type DecodedDataURL = {
	mediaType: string;
	isBase64: boolean;
	bytes: Uint8Array<ArrayBuffer>;
};

export const decodeDataURL = (url: string): DecodedDataURL | null => {
	const match = /^data:([^,]*?)(;base64)?,(.*)$/i.exec(url);
	if (!match) {
		return null;
	}
	const [, header, isBase64, payload] = match;
	try {
		const bytes = isBase64
			? Uint8Array.from(atob(payload), (char) => char.charCodeAt(0))
			: new TextEncoder().encode(decodeURIComponent(payload));
		return {
			mediaType: header.split(";")[0].trim(),
			isBase64: Boolean(isBase64),
			bytes,
		};
	} catch {
		return null;
	}
};
