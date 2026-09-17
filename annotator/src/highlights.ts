import type { HighlightItem, HighlightState } from "./protocol";

interface PlacedHighlight extends HighlightItem {
	node: HTMLDivElement;
	// Last rect the selector resolved to; used briefly after the element
	// disappears (for example mid re-render) so the shimmer does not flicker.
	lastRect?: DOMRect;
	missingSince?: number;
}

// How long a highlight keeps its last position after its selector stops
// matching before it is hidden.
const highlightGraceMs = 3000;
// Matches the fade-out duration of `.shimmer.done` in styles.
const doneFadeMs = 1300;

interface HighlightLayer {
	set(items: HighlightItem[], state: HighlightState): void;
	destroy(): void;
}

/**
 * Draws shimmering boxes over elements the agent is currently changing.
 * Highlights resolve their selector on every frame rather than holding a
 * node, so they follow re-renders, HMR swaps, and resizes.
 */
export function createHighlightLayer(
	doc: Document,
	win: Window,
	container: HTMLElement,
): HighlightLayer {
	const placed: PlacedHighlight[] = [];
	let loop = 0;
	let doneTimer = 0;

	const position = () => {
		const now = performance.now();
		for (const item of placed) {
			let target: Element | null = null;
			try {
				target = doc.querySelector(item.selector);
			} catch {
				target = null;
			}
			let rect = target?.getBoundingClientRect();
			if (rect && (rect.width > 0 || rect.height > 0)) {
				item.lastRect = rect;
				item.missingSince = undefined;
			} else {
				item.missingSince ??= now;
				rect =
					now - item.missingSince < highlightGraceMs
						? item.lastRect
						: undefined;
			}
			if (!rect) {
				item.node.style.display = "none";
				continue;
			}
			item.node.style.display = "block";
			item.node.style.left = `${rect.left - 2}px`;
			item.node.style.top = `${rect.top - 2}px`;
			item.node.style.width = `${rect.width + 4}px`;
			item.node.style.height = `${rect.height + 4}px`;
		}
	};

	const stop = () => {
		if (loop !== 0) {
			win.cancelAnimationFrame(loop);
			loop = 0;
		}
		if (doneTimer !== 0) {
			win.clearTimeout(doneTimer);
			doneTimer = 0;
		}
	};

	const clear = () => {
		stop();
		for (const item of placed.splice(0)) {
			item.node.remove();
		}
	};

	const set = (items: HighlightItem[], state: HighlightState) => {
		clear();
		for (const item of items) {
			const node = doc.createElement("div");
			node.className = state === "done" ? "shimmer done" : "shimmer";
			node.style.display = "none";
			container.append(node);
			placed.push({ ...item, node });
		}
		if (placed.length === 0) {
			return;
		}
		// Pending highlights must track layout changes the page makes on its
		// own (agent-driven HMR updates), so they run a frame loop while any
		// exist instead of piggybacking on scroll and resize events.
		const tick = () => {
			position();
			loop = win.requestAnimationFrame(tick);
		};
		loop = win.requestAnimationFrame(tick);
		if (state === "done") {
			doneTimer = win.setTimeout(clear, doneFadeMs);
		}
	};

	return { set, destroy: clear };
}
