import type { ViewportBox } from "./geometry";

/**
 * Creates an element with a class and attributes in one call, since the
 * overlay builds its UI by hand.
 */
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

/**
 * Lays an absolutely positioned node over a viewport box.
 */
export function placeOver(node: HTMLElement, box: ViewportBox): void {
	node.style.left = `${box.left}px`;
	node.style.top = `${box.top}px`;
	node.style.width = `${box.width}px`;
	node.style.height = `${box.height}px`;
}

/**
 * Decorations hang above a box's top-left corner and off its top-right
 * corner. Flags the edges where a child would leave the viewport so the
 * stylesheet can move it above the box or inside it. Call once the node
 * is placed and in the document; the box itself being clamped is not the
 * same test, since decorations reach further than the outline inset.
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
