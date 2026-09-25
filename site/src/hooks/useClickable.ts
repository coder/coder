import {
	type KeyboardEventHandler,
	type MouseEventHandler,
	type RefObject,
	type SyntheticEvent,
	useRef,
} from "react";

// Literally any object (ideally an HTMLElement) that has a .click method
type ClickableElement = {
	click: () => void;
};

/**
 * May be worth adding support for the 'spinbutton' role at some point, but that
 * will change the structure of the return result in a big way. Better to wait
 * until we actually need it.
 *
 * @see {@link https://www.w3.org/WAI/ARIA/apg/patterns/spinbutton/}
 */
export type ClickableAriaRole = "button" | "switch";

export type UseClickableResult<
	TElement extends ClickableElement = ClickableElement,
	TRole extends ClickableAriaRole = ClickableAriaRole,
> = Readonly<{
	ref: RefObject<TElement | null>;
	tabIndex: 0;
	role: TRole;
	onClick: MouseEventHandler<TElement>;
	onKeyDown: KeyboardEventHandler<TElement>;
	onKeyUp: KeyboardEventHandler<TElement>;
}>;

/**
 * True when the event's target is outside `currentTarget` in the DOM, as with
 * events React bubbles from portaled dialogs and menus.
 */
export const isFromPortal = (event: SyntheticEvent<unknown>): boolean =>
	event.currentTarget instanceof Node &&
	event.target instanceof Node &&
	!event.currentTarget.contains(event.target);

/**
 * Exposes props that let you turn traditionally non-interactive elements into
 * buttons.
 */
export const useClickable = <
	TElement extends ClickableElement,
	TRole extends ClickableAriaRole = ClickableAriaRole,
>(
	onClick: MouseEventHandler<TElement>,
	role?: TRole,
): UseClickableResult<TElement, TRole> => {
	const ref = useRef<TElement>(null);

	return {
		ref,
		onClick: (event) => {
			if (!isFromPortal(event)) {
				onClick(event);
			}
		},
		tabIndex: 0,
		role: (role ?? "button") as TRole,

		/*
		 * Mirrors native buttons: Enter activates on keydown (repeats while held),
		 * Space on keyup. Keys typed into descendants, including portaled dialogs,
		 * must not activate it.
		 */
		onKeyDown: (event) => {
			if (event.target !== event.currentTarget) {
				return;
			}
			if (event.key === "Enter") {
				ref.current?.click();
				event.stopPropagation();
			}
		},
		onKeyUp: (event) => {
			if (event.target !== event.currentTarget) {
				return;
			}
			if (event.key === " ") {
				ref.current?.click();
				event.stopPropagation();
			}
		},
	};
};
