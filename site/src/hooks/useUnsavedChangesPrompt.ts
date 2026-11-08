import { useEffect } from "react";
import { useBlocker } from "react-router";

type UnsavedChangesPromptState = {
	isOpen: boolean;
	onCancel: () => void;
	onConfirm: () => void;
};

/**
 * Warns the user before leaving while there are unsaved changes. Pairs a
 * `beforeunload` listener for hard navigations (tab close, refresh, address
 * bar) with `useBlocker` for in-app navigations. The browser owns the dialog
 * for hard navigations; the caller renders one for in-app navigations using
 * the returned state.
 */
export const useUnsavedChangesPrompt = (
	enabled: boolean,
): UnsavedChangesPromptState => {
	useEffect(() => {
		if (!enabled) return;
		const onBeforeUnload = (event: BeforeUnloadEvent) => {
			event.preventDefault();
			// Older browsers also require a return value to trigger the prompt.
			return "";
		};
		window.addEventListener("beforeunload", onBeforeUnload);
		return () => {
			window.removeEventListener("beforeunload", onBeforeUnload);
		};
	}, [enabled]);

	const blocker = useBlocker(
		({ currentLocation, nextLocation }) =>
			enabled && currentLocation.pathname !== nextLocation.pathname,
	);

	return {
		isOpen: blocker.state === "blocked",
		onCancel: () => blocker.reset?.(),
		onConfirm: () => blocker.proceed?.(),
	};
};
