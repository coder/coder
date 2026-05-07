import type { BuildInfoResponse } from "#/api/typesGenerated";

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

type PrereleaseFlag = "devel" | "rc" | undefined;

/** Classifies the dashboard build version for dev vs RC styling and experiments. */
export const getPrereleaseFlag = (
	input?: BuildInfoResponse,
): PrereleaseFlag => {
	// If no input is provided, return undefined.
	if (!input) {
		return undefined;
	}

	const version = input.version;

	// Check for dev version pattern (contains "-devel") or no version (v0.0.0)
	if (version.includes("-devel") || version === "v0.0.0") {
		return "devel";
	}

	// Check if the current build is a release candidate. Release
	// candidates have versions containing "-rc." (e.g. v2.32.0-rc.0,
	// v2.32.0-rc.1+abc123, v2.33.0-rc.1-devel+727ec00f7).
	if (version.includes("-rc.")) {
		return "rc";
	}
};
