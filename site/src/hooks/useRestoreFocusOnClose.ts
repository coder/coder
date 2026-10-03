import { useState } from "react";

const getFocusedElement = (): HTMLElement | null => {
	const active = document.activeElement;
	if (active instanceof HTMLElement && active !== document.body) {
		return active;
	}
	// Chrome moves focus to the body when a focused control becomes disabled,
	// so fall back to the topmost open dialog.
	const dialogs = document.querySelectorAll<HTMLElement>('[role="dialog"]');
	return dialogs[dialogs.length - 1] ?? null;
};

/** An `onCloseAutoFocus` handler body for dialogs that open without a Radix trigger. */
export const restoreFocusTo = (event: Event, element: HTMLElement | null) => {
	if (!element?.isConnected) {
		return;
	}
	const target = element.matches(":disabled")
		? element.closest<HTMLElement>('[role="dialog"]')
		: element;
	if (target) {
		event.preventDefault();
		target.focus();
	}
};

/**
 * Returns an `onCloseAutoFocus` handler that returns focus to the element
 * focused when `open` last turned true, or at mount for dialogs that mount
 * open. It captures during render, before the dialog takes focus.
 */
export const useRestoreFocusOnClose = (open = true) => {
	const [wasOpen, setWasOpen] = useState(false);
	const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(null);
	if (open !== wasOpen) {
		setWasOpen(open);
		if (open) {
			setReturnFocus(getFocusedElement());
		}
	}
	return (event: Event) => restoreFocusTo(event, returnFocus);
};
