import { el, placeOver } from "./dom";
import { outlineInset, viewportBox } from "./geometry";
import { type Annotation, annotationIdAttribute } from "./protocol";

type HeldComment = {
	annotation: Annotation;
	target: Element;
	outline: HTMLElement;
};

type HeldComments = {
	// Toolbar badge with the count; clicking it discards what is held.
	badge: HTMLButtonElement;
	count(): number;
	hold(annotation: Annotation, target: Element): void;
	// Hands over everything held, outlines cleared, to be sent together.
	take(): HeldComment[];
	// Drops everything held and unstamps the elements, as if never picked.
	discard(): void;
	reposition(): void;
};

/**
 * Comments held back with Shift+Send, sent together with the next plain
 * Send as one submission. Their elements keep a dashed outline meanwhile.
 */
export function createHeldComments(
	win: Window,
	layer: ShadowRoot,
): HeldComments {
	const doc = win.document;
	const held: HeldComment[] = [];

	const badge = el(doc, "button", "held-badge", {
		type: "button",
		"aria-label": "Discard held comments",
		"data-tip": "Comments waiting to be sent. Click to discard.",
	});
	badge.style.display = "none";
	const updateBadge = () => {
		badge.textContent = String(held.length);
		badge.style.display = held.length > 0 ? "flex" : "none";
	};

	const reposition = () => {
		for (const { target, outline } of held) {
			placeOver(
				outline,
				viewportBox(target.getBoundingClientRect(), win, outlineInset),
			);
		}
	};

	const take = () => {
		const items = held.splice(0);
		for (const item of items) {
			item.outline.remove();
		}
		updateBadge();
		return items;
	};

	const discard = () => {
		for (const item of take()) {
			item.target.removeAttribute(annotationIdAttribute);
		}
	};
	badge.addEventListener("click", (event) => {
		event.stopPropagation();
		discard();
	});

	return {
		badge,
		count: () => held.length,
		hold: (annotation, target) => {
			const outline = el(doc, "div", "held-outline", { "aria-hidden": "true" });
			layer.append(outline);
			held.push({ annotation, target, outline });
			reposition();
			updateBadge();
		},
		take,
		discard,
		reposition,
	};
}
