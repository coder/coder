import type { DraggableSyntheticListeners } from "@dnd-kit/core";

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
	onPointerDown: (event: React.PointerEvent) => {
		listeners?.onPointerDown?.(event);
		event.preventDefault();
	},
});
