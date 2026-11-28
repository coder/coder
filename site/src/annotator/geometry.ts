// How far outside an element its outline is drawn, so the border does not
// sit on the element's own edges. Shared by every state that outlines.
export const outlineInset = 4;

export type ViewportBox = {
	left: number;
	top: number;
	width: number;
	height: number;
};

/**
 * Expands `rect` by `inset` on every side and clamps the result to the
 * viewport, so an outline around an element larger than the screen (a
 * hero, a full-page section) still shows all four edges.
 */
export function viewportBox(
	rect: DOMRect,
	win: Pick<Window, "innerWidth" | "innerHeight">,
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
	};
}
