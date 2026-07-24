import { useEffect, useRef } from "react";

// Pixels of slack added when matching a file against the scroll offset, so a
// file counts as active while its sticky header is still pinned at the top.
const ACTIVE_FILE_SCROLL_THRESHOLD = 4;

// Minimal view of the @pierre/diffs CodeView instance handed to `onScroll`.
// `getRenderedItems` only returns the small set of currently virtualized
// items, so deriving the active file from it avoids scanning every file.
export interface ScrollViewer {
	getRenderedItems(): readonly { id: string }[];
	getTopForItem(id: string): number | undefined;
}

// The active file is the rendered item closest to the top edge that has
// already crossed it (largest top still at or above the fold). Exported for
// unit tests that pin the closest-to-top selection logic.
export function getActiveFile(
	scrollTop: number,
	viewer: ScrollViewer,
): string | undefined {
	const rendered = viewer.getRenderedItems();
	const limit = scrollTop + ACTIVE_FILE_SCROLL_THRESHOLD;
	let activePath: string | undefined;
	let activeTop = Number.NEGATIVE_INFINITY;
	for (const item of rendered) {
		const top = viewer.getTopForItem(item.id);
		if (top !== undefined && top <= limit && top > activeTop) {
			activeTop = top;
			activePath = item.id;
		}
	}
	return activePath ?? rendered[0]?.id;
}

/**
 * Reports the diff file scrolled to the top as the user scrolls. Returns a
 * CodeView `onScroll` handler. Tree-agnostic: callers decide what to do with
 * the active path (the diff viewer feeds it to the sidebar selection).
 */
export function useActiveFileTracking({
	enabled,
	onActiveFileChange,
}: {
	enabled: boolean;
	onActiveFileChange: (path: string) => void;
}) {
	const rafRef = useRef<number | null>(null);

	useEffect(() => {
		return () => {
			if (rafRef.current !== null) {
				cancelAnimationFrame(rafRef.current);
			}
		};
	}, []);

	return (scrollTop: number, viewer: ScrollViewer) => {
		if (!enabled) return;
		if (rafRef.current !== null) {
			cancelAnimationFrame(rafRef.current);
		}
		// Coalesce bursts of scroll events into one update per frame.
		rafRef.current = requestAnimationFrame(() => {
			rafRef.current = null;
			const next = getActiveFile(scrollTop, viewer);
			if (next) {
				onActiveFileChange(next);
			}
		});
	};
}
