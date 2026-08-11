import {
	defaultMetadataManager,
	useEmbeddedMetadata,
} from "#/hooks/useEmbeddedMetadata";

// The `ai-tasks-enabled` metadata mirrors the CODER_ENABLE_AI_TASKS deployment
// value and is the master switch for every Coder Tasks surface. The Go server
// always embeds it, so a missing value means the frontend is being served by
// Vite, Storybook, or a test runner. Those environments fall back to enabled so
// local development and stories keep rendering Tasks.
function isEnabled(value: boolean | undefined): boolean {
	return value ?? true;
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
