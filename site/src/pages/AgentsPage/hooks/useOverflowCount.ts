import { type RefObject, useLayoutEffect, useState } from "react";

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
 *   last visible width is cached here for fit decisions, which works
 *   because every item stays mounted at a stable index.
 * - A "+N" pill always renders as the last child (`visibility:
 *   hidden` when the count is 0) so its width can be read from the
 *   DOM instead of hardcoded.
 * - The container is the last child of its flex group, so the space
 *   from the container's left edge to the group's right edge is the
 *   space badges may occupy once growing siblings are capped. Growing
 *   siblings (the model pill) get priority: their remaining growth,
 *   read as the truncation deficit of their descendants, is reserved
 *   before badges claim space.
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

		// Hidden items report zero width, so fit decisions reuse the
		// width each item had while visible. Reset per effect run:
		// itemCount changes re-run the effect and invalidate indices.
		const lastVisibleWidths: number[] = [];

		const measure = () => {
			const children = container.children;
			const count = Math.min(itemCount, children.length);
			if (count === 0) {
				setOverflowCount(0);
				return;
			}

			const available =
				parent.getBoundingClientRect().right -
				container.getBoundingClientRect().left -
				siblingTruncationDeficit(parent, container);
			const gap = Number.parseFloat(
				getComputedStyle(container).columnGap || "0",
			);

			const widths: number[] = [];
			for (let i = 0; i < count; i++) {
				const width = children[i].getBoundingClientRect().width;
				if (width > 0) {
					lastVisibleWidths[i] = width;
				}
				widths.push(lastVisibleWidths[i] ?? 0);
			}

			// First pass: do all items fit without the pill?
			// +1px tolerance for subpixel rounding in getBoundingClientRect.
			let total = 0;
			for (let i = 0; i < count; i++) {
				total += widths[i] + (i > 0 ? gap : 0);
			}
			if (total <= available + 1) {
				setOverflowCount(0);
				return;
			}

			// Something overflows: reserve space for the pill, then
			// count how many leading items fit in what remains.
			const pill = children[children.length - 1];
			const pillWidth = pill ? pill.getBoundingClientRect().width : 0;
			const availableWithPill = available - pillWidth - gap;

			let visible = 0;
			let used = 0;
			for (let i = 0; i < count; i++) {
				used += widths[i] + (i > 0 ? gap : 0);
				if (used > availableWithPill + 1) {
					break;
				}
				visible++;
			}

			setOverflowCount(Math.max(count - visible, 1));
		};

		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(container);
		// The available space shifts when siblings (model selector,
		// workspace pill) grow or shrink without resizing the parent,
		// and when items hide or show without resizing the container.
		ro.observe(parent);
		for (const sibling of parent.children) {
			if (sibling !== container) {
				ro.observe(sibling);
			}
		}
		for (const child of container.children) {
			ro.observe(child);
		}

		const mo = new MutationObserver(measure);
		mo.observe(container, { childList: true });

		return () => {
			ro.disconnect();
			mo.disconnect();
		};
	}, [containerRef, itemCount]);

	return overflowCount;
}

// How much wider the container's siblings want to be: the total
// clipped overflow of their truncating descendants. Reserving this
// keeps sibling pills at full width in preference to inline badges;
// badges that lose the contest remain reachable in the +N popover.
function siblingTruncationDeficit(
	parent: HTMLElement,
	container: HTMLElement,
): number {
	let deficit = 0;
	for (const sibling of parent.children) {
		if (sibling === container) {
			continue;
		}
		for (const el of sibling.querySelectorAll("*")) {
			if (el instanceof HTMLElement) {
				deficit += Math.max(0, el.scrollWidth - el.clientWidth);
			}
		}
	}
	return deficit;
}
