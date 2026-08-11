import {
	defaultMetadataManager,
	useEmbeddedMetadata,
} from "#/hooks/useEmbeddedMetadata";

// The `ai-tasks-enabled` metadata mirrors the CODER_ENABLE_AI_TASKS deployment
// value and is the master switch for every Coder Tasks surface. It is only
// embedded by the Go server, so it is always missing when the frontend is
// served by Vite or Storybook. Those environments fall back to enabled so local
// development and stories keep rendering Tasks.
function isEnabled(value: boolean | undefined): boolean {
	return Boolean(
		value || process.env.NODE_ENV === "development" || process.env.STORYBOOK,
	);
}

export function useAITasksEnabled(): boolean {
	const { metadata } = useEmbeddedMetadata();
	return isEnabled(metadata["ai-tasks-enabled"].value);
}

// Module-scope equivalent of `useAITasksEnabled` for code that runs outside of
// React, such as route registration in `router.tsx`.
export function aiTasksEnabled(): boolean {
	return isEnabled(
		defaultMetadataManager.getMetadata()["ai-tasks-enabled"].value,
	);
}
