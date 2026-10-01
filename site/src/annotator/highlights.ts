import { outlineInset, viewportBox } from "./geometry";
import { annotationIdAttribute, type HighlightItem } from "./protocol";

type PlacedHighlight = {
	node: HTMLDivElement;
	// Element the item resolved to on a previous frame. Re-resolving only
	// when it leaves the document keeps the per-frame cost to a rect read.
	element?: Element;
	// Last rect the element had; used briefly after it disappears (for
	// example mid re-render) so the shimmer does not flicker.
	lastRect?: DOMRect;
	missingSince?: number;
	// Last box written to the node, so unchanged frames write nothing.
	lastBox?: string;
} & HighlightItem;

// How long a highlight keeps its last position after its element stops
// resolving before it is hidden.
const highlightGraceMs = 3000;

type HighlightLayer = {
	set(items: HighlightItem[]): void;
	// Draws a quiet ring for an annotation that was just sent, until the
	// dashboard replaces it with the working state or it times out.
	markPending(item: HighlightItem): void;
	destroy(): void;
};

// How long a just-sent annotation keeps its quiet ring when the agent
// never starts on it. Matches the dashboard's own grace period.
const pendingRingMs = 15_000;

/**
 * Draws shimmering boxes over elements the agent is currently changing.
 * Elements are looked up by the stamp the overlay put on them when the
 * comment was sent, so a same-shaped element on another page is never
 * mistaken for the annotated one, and re-resolved whenever they leave the
 * document so the boxes follow re-renders, HMR swaps, and resizes.
 */
export function createHighlightLayer(
	doc: Document,
	win: Window,
	container: HTMLElement,
): HighlightLayer {
	const placed: PlacedHighlight[] = [];
	let loop = 0;

	// The stamp is authoritative. The selector only fills in while the
	// stamped node is missing (mid re-render, HMR swap) and the preview is
	// still on the annotated page.
	const resolve = (item: PlacedHighlight): Element | null => {
		if (item.element?.isConnected) {
			return item.element;
		}
		item.element = undefined;
		const stamped = querySafely(
			doc,
			`[${annotationIdAttribute}="${cssString(item.id)}"]`,
		);
		if (stamped) {
			item.element = stamped;
			return stamped;
		}
		if (!samePage(item.url, win.location.href)) {
			return null;
		}
		const fallback = querySafely(doc, item.selector);
		item.element = fallback ?? undefined;
		return fallback;
	};

	const position = () => {
		const now = performance.now();
		for (const item of placed) {
			const target = resolve(item);
			let rect = target?.getBoundingClientRect();
			if (rect && (rect.width > 0 || rect.height > 0)) {
				item.lastRect = rect;
				item.missingSince = undefined;
			} else if (!samePage(item.url, win.location.href)) {
				// Off the annotated page: nothing to hold a position for.
				rect = undefined;
			} else {
				item.missingSince ??= now;
				rect =
					now - item.missingSince < highlightGraceMs
						? item.lastRect
						: undefined;
			}
			if (!rect) {
				if (item.lastBox !== "") {
					item.node.style.display = "none";
					item.lastBox = "";
				}
				continue;
			}
			const box = viewportBox(rect, win, outlineInset);
			const key = `${box.left},${box.top},${box.width},${box.height}`;
			if (key === item.lastBox) {
				continue;
			}
			item.lastBox = key;
			item.node.style.display = "block";
			item.node.style.left = `${box.left}px`;
			item.node.style.top = `${box.top}px`;
			item.node.style.width = `${box.width}px`;
			item.node.style.height = `${box.height}px`;
			// The beam's rotating gradient must cover the box's corners at
			// any angle, so it is sized from the diagonal.
			item.node.style.setProperty(
				"--diagonal",
				`${Math.ceil(Math.hypot(box.width, box.height))}px`,
			);
		}
	};

	const stop = () => {
		if (loop !== 0) {
			win.cancelAnimationFrame(loop);
			loop = 0;
		}
	};

	const clear = () => {
		stop();
		for (const item of placed.splice(0)) {
			item.node.remove();
		}
	};

	// Highlights are the only consumer of the stamps, so they go when the
	// highlights are cleared rather than lingering in the page's DOM. Only
	// stamps for known annotation ids are touched: the attribute is
	// namespaced, but an app could still use it for its own purposes.
	const unstamp = (ids: string[]) => {
		for (const id of ids) {
			const node = querySafely(
				doc,
				`[${annotationIdAttribute}="${cssString(id)}"]`,
			);
			node?.removeAttribute(annotationIdAttribute);
		}
	};

	const place = (item: HighlightItem, className: string) => {
		const node = doc.createElement("div");
		node.className = className;
		node.style.display = "none";
		const beam = doc.createElement("div");
		beam.className = "beam";
		node.append(beam);
		container.append(node);
		placed.push({ ...item, node, lastBox: "" });
	};

	// Highlights must track layout changes the page makes on its own
	// (agent-driven HMR updates), so they run a frame loop while any exist
	// instead of piggybacking on scroll and resize events.
	const ensureLoop = () => {
		if (loop !== 0 || placed.length === 0) {
			return;
		}
		const tick = () => {
			position();
			loop = win.requestAnimationFrame(tick);
		};
		loop = win.requestAnimationFrame(tick);
	};

	let pendingTimer = 0;

	const set = (items: HighlightItem[]) => {
		const previous = placed.map((item) => item.id);
		clear();
		win.clearTimeout(pendingTimer);
		if (items.length === 0) {
			unstamp(previous);
		}
		for (const item of items) {
			place(item, "shimmer");
		}
		ensureLoop();
	};

	const markPending = (item: HighlightItem) => {
		// Only quiet rings can be pending; once the dashboard has taken over
		// with the working state, its message is the source of truth.
		if (
			placed.some((existing) => !existing.node.classList.contains("pending"))
		) {
			return;
		}
		place(item, "shimmer pending");
		ensureLoop();
		win.clearTimeout(pendingTimer);
		pendingTimer = win.setTimeout(() => set([]), pendingRingMs);
	};

	return {
		set,
		markPending,
		destroy: () => {
			const ids = placed.map((item) => item.id);
			clear();
			win.clearTimeout(pendingTimer);
			unstamp(ids);
		},
	};
}

function querySafely(doc: Document, selector: string): Element | null {
	try {
		return doc.querySelector(selector);
	} catch {
		return null;
	}
}

// Escapes a value for use inside a double-quoted attribute selector.
function cssString(value: string): string {
	return value.replaceAll(/["\\]/g, "\\$&").replaceAll("\n", "\\a ");
}

function samePage(a: string, b: string): boolean {
	try {
		const left = new URL(a);
		const right = new URL(b);
		return left.origin === right.origin && left.pathname === right.pathname;
	} catch {
		return false;
	}
}
