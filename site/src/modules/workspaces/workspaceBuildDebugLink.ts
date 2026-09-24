// The contract between the workspace page, which renders the debug action,
// and the agents create page, which consumes it.

// Failed build ID; the create page fetches the build and logs so the link
// stays shareable.
export const debugWorkspaceBuildSearchParam = "debug_workspace_build";

export const buildDebugWorkspaceBuildPath = (buildId: string): string =>
	`/agents?${debugWorkspaceBuildSearchParam}=${encodeURIComponent(buildId)}`;

// Written by the workspace page when the user clicks the action and taken by
// the create page, so only a real click sends without confirmation. A pasted
// or replayed link prefills the chat instead.
/** @internal Exported for testing. */
export const debugWorkspaceBuildIntentStorageKey =
	"agents.debug-workspace-build-intent";
// Bounds how long a click can wait before a matching tab loses auto-send.
/** @internal Exported for testing. */
export const debugWorkspaceBuildIntentMaxAgeMs = 5 * 60 * 1000;

type DebugWorkspaceBuildIntent = {
	buildId: string;
	at: number;
};

export const storeDebugWorkspaceBuildIntent = (buildId: string): void => {
	const intent: DebugWorkspaceBuildIntent = { buildId, at: Date.now() };
	localStorage.setItem(
		debugWorkspaceBuildIntentStorageKey,
		JSON.stringify(intent),
	);
};

/**
 * Removes the stored intent for this build and reports whether it was fresh.
 * An intent for another build is left for that build's tab.
 */
export const takeDebugWorkspaceBuildIntent = (buildId: string): boolean => {
	const raw = localStorage.getItem(debugWorkspaceBuildIntentStorageKey);
	if (raw === null) {
		return false;
	}
	let intent: unknown;
	try {
		intent = JSON.parse(raw);
	} catch {
		return false;
	}
	if (
		typeof intent !== "object" ||
		intent === null ||
		!("buildId" in intent) ||
		intent.buildId !== buildId ||
		!("at" in intent) ||
		typeof intent.at !== "number"
	) {
		return false;
	}
	localStorage.removeItem(debugWorkspaceBuildIntentStorageKey);
	return Date.now() - intent.at < debugWorkspaceBuildIntentMaxAgeMs;
};
