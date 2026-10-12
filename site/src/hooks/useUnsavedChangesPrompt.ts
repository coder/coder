import { useEffect, useState } from "react";
import { useBlocker } from "react-router";
import { useRestoreFocusOnClose } from "./useRestoreFocusOnClose";

type UnsavedChangesPromptState = {
	isOpen: boolean;
	onCancel: () => void;
	onConfirm: () => void;
	/**
	 * Pass to the prompt dialog. After Stay, it returns focus to the element
	 * that had it when the navigation was blocked.
	 */
	onCloseAutoFocus: (event: Event) => void;
};

/**
 * Warns the user before leaving while there are unsaved changes. Pairs a
 * `beforeunload` listener for hard navigations (tab close, refresh, address
 * bar) with `useBlocker` for in-app navigations. The browser owns the dialog
 * for hard navigations; the caller renders one for in-app navigations using
 * the returned state.
 *
 * An open prompt closes when `enabled` turns false, or when `reason` changes,
 * so callers can word the prompt for the reason it opened.
 */
export const useUnsavedChangesPrompt = (
	enabled: boolean,
	reason?: string,
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
	const isBlocked = blocker.state === "blocked";
	const onCloseAutoFocus = useRestoreFocusOnClose(isBlocked);
	const [blockedReason, setBlockedReason] = useState<string>();
	const [wasBlocked, setWasBlocked] = useState(false);
	if (isBlocked !== wasBlocked) {
		setWasBlocked(isBlocked);
		setBlockedReason(reason);
	}
	// A prompt opened while enabled must not outlive the reason for it.
	const resetBlocker = blocker.reset;
	const isStale = isBlocked && (!enabled || blockedReason !== reason);
	useEffect(() => {
		if (isStale) {
			resetBlocker?.();
		}
	}, [isStale, resetBlocker]);

	return {
		isOpen: isBlocked,
		onCancel: () => blocker.reset?.(),
		onConfirm: () => blocker.proceed?.(),
		onCloseAutoFocus,
	};
};
