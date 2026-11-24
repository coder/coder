import type { ViewportBox } from "./geometry";

/** Creates overlay markup in the supplied page document, not the caller's realm. */
export function el<K extends keyof HTMLElementTagNameMap>(
	doc: Document,
	tag: K,
	className?: string,
	attributes: Record<string, string> = {},
): HTMLElementTagNameMap[K] {
	const node = doc.createElement(tag);
	if (className) {
		node.className = className;
	}
	for (const [name, value] of Object.entries(attributes)) {
		node.setAttribute(name, value);
	}
	return node;
}

/** Positions an overlay node in viewport coordinates. */
export function placeOver(node: HTMLElement, box: ViewportBox): void {
	node.style.left = `${box.left}px`;
	node.style.top = `${box.top}px`;
	node.style.width = `${box.width}px`;
	node.style.height = `${box.height}px`;
}

/**
 * Flags overflowing decorations after the node is placed in the document.
 * The decorations can extend beyond an already-clamped outline.
 */
export function flagCutEdges(
	node: HTMLElement,
	viewport: Pick<Window, "innerWidth">,
): void {
	const anyChild = (cut: (rect: DOMRect) => boolean) =>
		Array.from(node.children).some((child) =>
			cut(child.getBoundingClientRect()),
		);
	node.classList.remove("at-top", "at-right");
	node.classList.toggle(
		"at-right",
		anyChild((rect) => rect.right > viewport.innerWidth),
	);
	// Measured after the right edge has settled: a decoration moved above
	// the box may now be the one leaving the viewport.
	node.classList.toggle(
		"at-top",
		anyChild((rect) => rect.top < 0),
	);
}
