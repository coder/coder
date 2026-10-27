import { useRef } from "react";

const canFocus = (
	button: HTMLButtonElement | null,
): button is HTMLButtonElement =>
	Boolean(button?.isConnected && !button.disabled);

/**
 * Returns focus to the button that opened a dialog. Dialogs opened without a
 * Radix DialogTrigger would otherwise return focus to the document body. An
 * unmounted or disabled opener falls back to the primary, then the secondary
 * fallback button, which callers register as callback refs.
 */
export const useDialogFocusReturn = () => {
	const triggerRef = useRef<HTMLButtonElement | null>(null);
	const primaryFallbackRef = useRef<HTMLButtonElement | null>(null);
	const secondaryFallbackRef = useRef<HTMLButtonElement | null>(null);
	/** For openers handed a button, since event.currentTarget is null after an await. */
	const rememberTriggerButton = (button: HTMLButtonElement) => {
		triggerRef.current = button;
	};
	const rememberTrigger = (event: React.SyntheticEvent<HTMLButtonElement>) => {
		rememberTriggerButton(event.currentTarget);
	};
	const restoreFocus = (event: Event) => {
		// An array find here would stop the React Compiler memoizing this closure.
		let target = triggerRef.current;
		if (!canFocus(target)) {
			target = primaryFallbackRef.current;
		}
		if (!canFocus(target)) {
			target = secondaryFallbackRef.current;
		}
		if (canFocus(target)) {
			event.preventDefault();
			target.focus();
		}
	};
	return {
		setPrimaryFallback: (button: HTMLButtonElement | null) => {
			primaryFallbackRef.current = button;
		},
		setSecondaryFallback: (button: HTMLButtonElement | null) => {
			secondaryFallbackRef.current = button;
		},
		rememberTrigger,
		rememberTriggerButton,
		restoreFocus,
	};
};
