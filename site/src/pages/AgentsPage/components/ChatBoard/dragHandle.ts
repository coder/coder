import type { DraggableSyntheticListeners } from "@dnd-kit/core";
import { type PointerEvent as ReactPointerEvent, useEffect } from "react";

/**
 * dnd-kit listeners for a drag handle. After dnd-kit has seen the pointerdown
 * (it ignores events whose default is already prevented), preventing the
 * default suppresses the compatibility mousedown, so the browser never starts
 * a text selection or a link drag from the handle. Clicks still fire.
 */
export const dragHandleListeners = (
	listeners: DraggableSyntheticListeners | undefined,
) => ({
	...listeners,
	onPointerDown: (event: ReactPointerEvent) => {
		listeners?.onPointerDown?.(event);
		event.preventDefault();
	},
});

/**
 * Blocks selections from starting anywhere while a drag is active. dnd-kit
 * clears ranges on selectionchange, but WebKit keeps extending a selection
 * gesture it already started; refusing selectstart stops that at the source.
 */
export const useBlockSelectionWhileDragging = (dragging: boolean) => {
	useEffect(() => {
		if (!dragging) return;
		const block = (event: Event) => event.preventDefault();
		document.addEventListener("selectstart", block);
		return () => document.removeEventListener("selectstart", block);
	}, [dragging]);
};
