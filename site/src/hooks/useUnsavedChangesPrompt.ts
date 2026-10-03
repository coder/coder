import { useEffect, useState } from "react";
import { useBlocker } from "react-router";

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

const getFocusedElement = (): HTMLElement | null => {
	const active = document.activeElement;
	if (active instanceof HTMLElement && active !== document.body) {
		return active;
	}
	// Chrome moves focus to the body when a focused control becomes disabled.
	const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"]');
	return dialogs[dialogs.length - 1] ?? null;
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
	// Captured in the render that opens the prompt, before the prompt takes
	// focus, and kept until the next one so the closing prompt can read it.
	const [wasBlocked, setWasBlocked] = useState(false);
	const [blockedState, setBlockedState] = useState<{
		reason: string | undefined;
		returnFocus: HTMLElement | null;
	}>();
	if (isBlocked !== wasBlocked) {
		setWasBlocked(isBlocked);
		if (isBlocked) {
			setBlockedState({ reason, returnFocus: getFocusedElement() });
		}
	}
	// A prompt opened while enabled must not outlive the reason for it.
	const resetBlocker = blocker.reset;
	const isStale = isBlocked && (!enabled || blockedState?.reason !== reason);
	useEffect(() => {
		if (isStale) {
			resetBlocker?.();
		}
	}, [isStale, resetBlocker]);

	return {
		isOpen: isBlocked,
		onCancel: () => blocker.reset?.(),
		onConfirm: () => blocker.proceed?.(),
		onCloseAutoFocus: (event) => {
			const element = blockedState?.returnFocus;
			if (!element?.isConnected) {
				return;
			}
			// A control disabled since then cannot take focus; its dialog can.
			const target = element.matches(":disabled")
				? element.closest<HTMLElement>('[role="dialog"]')
				: element;
			if (target) {
				event.preventDefault();
				target.focus();
			}
		},
	};
};
