// Link contract between the workspace page's debug action and the agents
// create page.

// Failed build ID; the create page fetches the build and logs so the link
// stays shareable.
export const debugWorkspaceBuildSearchParam = "debug_workspace_build";

export const buildDebugWorkspaceBuildPath = (buildId: string): string =>
	`/agents?${debugWorkspaceBuildSearchParam}=${encodeURIComponent(buildId)}`;
