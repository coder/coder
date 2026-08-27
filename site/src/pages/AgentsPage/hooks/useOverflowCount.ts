import { type RefObject, useLayoutEffect, useState } from "react";

// Tolerance for getBoundingClientRect subpixel rounding and the
// integer rounding of scrollWidth/clientWidth.
const TOLERANCE_PX = 1;

// Last visible width of each overflow-managed element. Hidden items
// report zero width, so fit decisions reuse the width an item had
// while visible. Keyed by element identity (module-wide is safe:
// elements are unique per hook instance) so the cache survives both
// re-renders and item count changes.
const lastVisibleWidths = new WeakMap<Element, number>();

/**
 * Number of leading items whose widths, plus the gaps between them,
 * fit within the budget.
 */
export function countThatFit(
	widths: readonly number[],
	gap: number,
	budget: number,
): number {
	let used = 0;
	for (let i = 0; i < widths.length; i++) {
		used += widths[i] + (i > 0 ? gap : 0);
		if (used > budget + TOLERANCE_PX) {
			return i;
		}
	}
	return widths.length;
}

/**
 * Overflow count for one measured snapshot: zero when every item fits
 * in the available space, otherwise how many trailing items must move
 * into the "+N" pill, whose width is reserved from the budget.
 */
export function computeOverflowCount(snapshot: {
	widths: readonly number[];
	gap: number;
	available: number;
	pillWidth: number;
}): number {
	const { widths, gap, available, pillWidth } = snapshot;
	if (countThatFit(widths, gap, available) === widths.length) {
		return 0;
	}
	const visible = countThatFit(widths, gap, available - pillWidth - gap);
	// Defensive floor: the second budget is strictly smaller, so at
	// least one item always overflows once the first pass fails.
	return Math.max(widths.length - visible, 1);
}

/**
 * Observes a flex container whose children are laid out as:
 *
 *   [item₀] [item₁] … [itemₙ₋₁] [pill]
 *
 * and reports how many of the first `itemCount` children do not fit
 * in the space available to the container. The count updates
 * automatically when layout or children change.
 *
 * Contract with the caller:
 *
 * - Overflowed items are hidden with `display: none` so they release
 *   their layout space (letting sibling pills grow into it). Their
 *   last visible width is cached per element for fit decisions.
 * - A "+N" pill always renders as the last child (`visibility:
 *   hidden` when the count is 0) so its width can be read from the
 *   DOM instead of hardcoded.
 * - The container is the last child of its flex group, so the space
 *   from the container's left edge to the group's right edge (LTR is
 *   assumed) is the space items may occupy once growing siblings are
 *   capped. Growing siblings (the model pill) get priority: their
 *   remaining growth, read as the truncation deficit of their
 *   descendants, is reserved before items claim space.
 */
export function useOverflowCount(
	containerRef: RefObject<HTMLElement | null>,
	itemCount: number,
): number {
	const [overflowCount, setOverflowCount] = useState(0);

	useLayoutEffect(() => {
		const container = containerRef.current;
		const parent = container?.parentElement;
		if (!container || !parent) {
			return;
		}

		const measure = () => {
			const children = container.children;
			const count = Math.min(itemCount, children.length);
			if (count === 0) {
				setOverflowCount(0);
				return;
			}

			const widths: number[] = [];
			for (let i = 0; i < count; i++) {
				const child = children[i];
				const width = child.getBoundingClientRect().width;
				if (width > 0) {
					lastVisibleWidths.set(child, width);
				}
				widths.push(lastVisibleWidths.get(child) ?? 0);
			}

			const pill = children[children.length - 1];
			setOverflowCount(
				computeOverflowCount({
					widths,
					gap: Number.parseFloat(getComputedStyle(container).columnGap || "0"),
					available:
						parent.getBoundingClientRect().right -
						container.getBoundingClientRect().left -
						siblingTruncationDeficit(parent, container),
					pillWidth: pill ? pill.getBoundingClientRect().width : 0,
				}),
			);
		};

		const ro = new ResizeObserver(measure);
		// The available space shifts when siblings (model selector,
		// workspace pill) grow or shrink without resizing the parent,
		// and when items hide or show without resizing the container.
		const observeAll = () => {
			ro.observe(container);
			ro.observe(parent);
			for (const sibling of parent.children) {
				if (sibling !== container) {
					ro.observe(sibling);
				}
			}
			for (const child of container.children) {
				ro.observe(child);
			}
		};

		measure();
		observeAll();

		// Re-attach in case a child was replaced in place; observing an
		// already-observed element is a no-op.
		const mo = new MutationObserver(() => {
			observeAll();
			measure();
		});
		mo.observe(container, { childList: true });

		return () => {
			ro.disconnect();
			mo.disconnect();
		};
	}, [containerRef, itemCount]);

	return overflowCount;
}

// How much wider the container's siblings want to be: the widest
// clipped overflow among each sibling's truncating descendants.
// Reserving this keeps sibling pills at full width in preference to
// inline items; items that lose the contest remain reachable in the
// +N popover. Only elements that actually clip (overflow-x hidden or
// clip) are counted, and only the widest one per sibling, so nested
// wrappers around one truncating label cannot double-count.
function siblingTruncationDeficit(
	parent: HTMLElement,
	container: HTMLElement,
): number {
	let deficit = 0;
	for (const sibling of parent.children) {
		if (sibling === container || !(sibling instanceof HTMLElement)) {
			continue;
		}
		let widest = 0;
		for (const el of [sibling, ...sibling.querySelectorAll("*")]) {
			if (!(el instanceof HTMLElement)) {
				continue;
			}
			const overflowX = getComputedStyle(el).overflowX;
			if (overflowX !== "hidden" && overflowX !== "clip") {
				continue;
			}
			const clipped = el.scrollWidth - el.clientWidth;
			if (clipped > TOLERANCE_PX) {
				widest = Math.max(widest, clipped);
			}
		}
		deficit += widest;
	}
	return deficit;
}
