import { type RefObject, useLayoutEffect, useState } from "react";

/**
 * Observes a flex container whose children are laid out as:
 *
 *   [item₀] [item₁] … [itemₙ₋₁] [pill]
 *
 * and reports how many of the first `itemCount` children overflow
 * past the container's visible width. The count updates
 * automatically when the container resizes or children change.
 *
 * The caller should always render a "+N" pill as the last child
 * (using `visibility: hidden` when the count is 0) so its layout
 * space is permanently reserved. The hook reads the pill's actual
 * rendered width and the container's CSS `gap` from the DOM, so
 * there are no hardcoded sizing assumptions.
 */
export function useOverflowCount(
	containerRef: RefObject<HTMLElement | null>,
	itemCount: number,
): number {
	const [overflowCount, setOverflowCount] = useState(0);

	useLayoutEffect(() => {
		const container = containerRef.current;
		if (!container) {
			return;
		}

		const measure = () => {
			const children = container.children;
			const count = Math.min(itemCount, children.length);
			if (count === 0) {
				setOverflowCount(0);
				return;
			}

			const containerRight = container.getBoundingClientRect().right;

			// First pass: check if all items fit at full width.
			// If so, no pill needed and we're done.
			// +1px tolerance for subpixel rounding in getBoundingClientRect.
			let allFit = true;
			for (let i = 0; i < count; i++) {
				if (children[i].getBoundingClientRect().right > containerRight + 1) {
					allFit = false;
					break;
				}
			}

			if (allFit) {
				setOverflowCount(0);
				return;
			}

			// Something genuinely overflows. Reserve space for the
			// pill (last child) so it won't be clipped. Read its
			// width and the container gap from the DOM rather than
			// hardcoding values that break under font scaling or
			// double-digit overflow counts.
			const pill = children[children.length - 1];
			const pillWidth = pill ? pill.getBoundingClientRect().width : 0;
			const gap = Number.parseFloat(
				getComputedStyle(container).columnGap || "0",
			);
			const effectiveRight = containerRight - pillWidth - gap;

			// +1px tolerance for subpixel rounding in getBoundingClientRect.
			let hidden = 0;
			for (let i = 0; i < count; i++) {
				if (children[i].getBoundingClientRect().right > effectiveRight + 1) {
					hidden++;
				}
			}

			setOverflowCount(Math.max(hidden, 1));
		};

		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(container);

		const mo = new MutationObserver(measure);
		mo.observe(container, { childList: true });

		return () => {
			ro.disconnect();
			mo.disconnect();
		};
	}, [containerRef, itemCount]);

	return overflowCount;
}
