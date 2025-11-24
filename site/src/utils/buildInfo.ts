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

// Check if the current build is a development build.
// Development builds have versions containing "-devel" or "v0.0.0".
// This matches the backend's buildinfo.IsDev() logic.
export const isDevBuild = (input: BuildInfoResponse): boolean => {
	const version = input.version;
	if (!version) {
		return false;
	}

	// Check for dev version pattern (contains "-devel") or no version (v0.0.0)
	return version.includes("-devel") || version === "v0.0.0";
};
