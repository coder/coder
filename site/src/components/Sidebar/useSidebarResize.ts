import { useCallback, useRef, useState } from "react";

const EXPANDED_WIDTH = 240;
const COLLAPSED_WIDTH = 64;

function readCollapsed(key: string): boolean {
	try {
		return localStorage.getItem(key) === "collapsed";
	} catch {
		return false;
	}
}

function persistCollapsed(key: string, collapsed: boolean): void {
	try {
		localStorage.setItem(key, collapsed ? "collapsed" : "expanded");
	} catch {
		// Silently ignore write failures.
	}
}

interface UseSidebarResizeReturn {
	width: number;
	collapsed: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

/**
 * Two-state sidebar that drags smoothly by writing directly to the
 * DOM during pointermove (no React re-renders). State is synced
 * once on pointerup so collapsed mode and localStorage update.
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
): UseSidebarResizeReturn {
	const [collapsed, setCollapsed] = useState(() => readCollapsed(storageKey));
	const containerRef = useRef<HTMLElement | null>(null);

	const expand = useCallback(() => {
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey]);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();

			const container =
				(e.currentTarget as HTMLElement).closest<HTMLElement>(
					"[data-sidebar-container]",
				) ?? (e.currentTarget as HTMLElement).parentElement;
			if (!container) return () => {};

			containerRef.current = container;
			const startLeft = container.getBoundingClientRect().left;
			const startWidth = container.getBoundingClientRect().width;
			let moved = false;

			// Kill the CSS transition so DOM writes are instant.
			container.style.transition = "none";

			const handlePointerMove = (moveEvent: PointerEvent) => {
				moved = true;
				const rawWidth = moveEvent.clientX - startLeft;
				const clamped = Math.max(
					COLLAPSED_WIDTH,
					Math.min(rawWidth, EXPANDED_WIDTH),
				);
				container.style.width = `${clamped}px`;
			};

			const cleanup = () => {
				document.removeEventListener("pointermove", handlePointerMove);
				document.removeEventListener("pointerup", cleanup);
				document.body.style.cursor = "";
				document.body.style.userSelect = "";

				const finalWidth = container.getBoundingClientRect().width;

				// Any drag movement in the correct direction completes
				// the transition. No large threshold needed.
				let shouldCollapse: boolean;
				if (!moved) {
					// Click without drag — toggle.
					shouldCollapse = !collapsed;
				} else {
					shouldCollapse = finalWidth < startWidth;
				}
				const snapWidth = shouldCollapse ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

				container.style.transition = "";
				container.style.width = `${snapWidth}px`;

				setCollapsed(shouldCollapse);
				persistCollapsed(storageKey, shouldCollapse);
				containerRef.current = null;
			};
			document.body.style.cursor = "col-resize";
			document.body.style.userSelect = "none";
			document.addEventListener("pointermove", handlePointerMove);
			document.addEventListener("pointerup", cleanup);

			return cleanup;
		},
		[collapsed, storageKey],
	);

	const width = collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

	return { width, collapsed, expand, onDragStart };
}
