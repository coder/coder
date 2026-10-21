/**
 * `onPointerDown` for a drag handle. dnd-kit's own listener on the handle
 * has already seen the event; preventing the default then suppresses the
 * compatibility mousedown, so the browser never starts a text selection or
 * a link drag from the handle. Clicks still fire.
 */
export const preventHandleSelection = (event: React.PointerEvent) =>
	event.preventDefault();
