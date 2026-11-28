import { el, flagCutEdges, placeOver } from "./dom";
import { elementLabel } from "./elementLabel";
import { outlineInset, viewportBox } from "./geometry";
import { sparklesIcon } from "./icons";

type PickOutline = {
	element: HTMLElement;
	// Outlines the element and names it, or hides the outline for null.
	follow(target: Element | null): void;
};

/**
 * The dashed outline that follows the pointer while picking and stays on
 * the element a comment is being written for.
 */
export function createPickOutline(win: Window): PickOutline {
	const doc = win.document;
	const element = el(doc, "div", "highlight", { "aria-hidden": "true" });
	const label = el(doc, "span", "highlight-label");
	const badge = el(doc, "span", "highlight-badge");
	badge.innerHTML = sparklesIcon;
	element.append(label, badge);

	return {
		element,
		follow: (target) => {
			if (!target) {
				element.style.display = "none";
				return;
			}
			// Drawn just outside the element so the dashed border does not sit
			// on top of its edges, and kept within the viewport so the outline
			// of an oversized element is still visible.
			placeOver(
				element,
				viewportBox(target.getBoundingClientRect(), win, outlineInset),
			);
			element.style.display = "block";
			label.textContent = elementLabel(target);
			flagCutEdges(element, win);
		},
	};
}
