import type { BuildInfoResponse } from "api/typesGenerated";

let CACHED_BUILD_INFO: BuildInfoResponse | undefined;

// During the build process, we inject the build info into the HTML
export const getStaticBuildInfo = () => {
	if (CACHED_BUILD_INFO) {
		return CACHED_BUILD_INFO;
	}

	const buildInfoJson = document
		.querySelector("meta[property=build-info]")
		?.getAttribute("content");

	if (buildInfoJson) {
		try {
			CACHED_BUILD_INFO = JSON.parse(buildInfoJson) as BuildInfoResponse;
		} catch {
			return undefined;
		}
	}

	return CACHED_BUILD_INFO;
};
