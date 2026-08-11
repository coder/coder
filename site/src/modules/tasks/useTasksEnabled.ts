import {
	defaultMetadataManager,
	useEmbeddedMetadata,
} from "#/hooks/useEmbeddedMetadata";

// The `tasks-enabled` metadata is the master switch for every Coder Tasks
// surface. It is only embedded by the Go server, so it is always missing when
// the frontend is served by Vite or Storybook. Those environments fall back to
// enabled so local development and stories keep rendering Tasks.
function isEnabled(value: boolean | undefined): boolean {
	return Boolean(
		value || process.env.NODE_ENV === "development" || process.env.STORYBOOK,
	);
}

export function useTasksEnabled(): boolean {
	const { metadata } = useEmbeddedMetadata();
	return isEnabled(metadata["tasks-enabled"].value);
}

// Module-scope equivalent of `useTasksEnabled` for code that runs outside of
// React, such as route registration in `router.tsx`.
export function tasksEnabled(): boolean {
	return isEnabled(defaultMetadataManager.getMetadata()["tasks-enabled"].value);
}
