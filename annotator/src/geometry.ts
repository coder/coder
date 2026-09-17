// How far outside an element its outline is drawn, so the border does not
// sit on the element's own edges. Shared by every state that outlines.
export const outlineInset = 4;

export interface ViewportBox {
	left: number;
	top: number;
	width: number;
	height: number;
	// Edges that were pulled in to stay on screen. Decorations hanging
	// off those edges need to move inside the box.
	clampedTop: boolean;
	clampedRight: boolean;
}

/**
 * Expands `rect` by `inset` on every side and clamps the result to the
 * viewport, so an outline around an element larger than the screen (a
 * hero, a full-page section) still shows all four edges.
 */
export function viewportBox(
	rect: DOMRect,
	win: Window,
	inset: number,
): ViewportBox {
	const left = Math.max(inset, rect.left - inset);
	const top = Math.max(inset, rect.top - inset);
	const right = Math.min(win.innerWidth - inset, rect.right + inset);
	const bottom = Math.min(win.innerHeight - inset, rect.bottom + inset);
	return {
		left,
		top,
		width: Math.max(0, right - left),
		height: Math.max(0, bottom - top),
		clampedTop: top > rect.top - inset,
		clampedRight: right < rect.right + inset,
	};
}
