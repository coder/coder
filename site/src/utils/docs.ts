import { getStaticBuildInfo } from "./buildInfo";

function defaultDocsUrl(): string {
	const docsUrl = "https://coder.com/docs";
	// If we can get the specific version, we want to include that in default docs URL.
	let version = getStaticBuildInfo()?.version;
	if (!version) {
		return docsUrl;
	}

	// Strip build metadata after '+', then remove a '-devel' suffix
	// if present. Preserve '-rc.X' suffixes so versioned docs links
	// point at the correct release candidate.
	version = version.split("+")[0].replace(/-devel$/, "");
	if (version === "v0.0.0") {
		return docsUrl;
	}
	return `${docsUrl}/@${version}`;
}

// Add cache to avoid DOM reading all the time
let CACHED_DOCS_URL: string | undefined;

export const docs = (path: string) => {
	return `${getBaseDocsURL()}${path}`;
};

const getBaseDocsURL = () => {
	if (!CACHED_DOCS_URL) {
		const docsUrl = document
			.querySelector<HTMLMetaElement>('meta[property="docs-url"]')
			?.getAttribute("content");

		const isValidDocsURL = docsUrl && isURL(docsUrl);
		CACHED_DOCS_URL = isValidDocsURL ? docsUrl : defaultDocsUrl();
	}
	return CACHED_DOCS_URL;
};

const isURL = (value: string) => {
	try {
		new URL(value);
		return true;
	} catch {
		return false;
	}
};
