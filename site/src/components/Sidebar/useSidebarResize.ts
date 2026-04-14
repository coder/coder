import { useCallback, useRef, useState } from "react";

const EXPANDED_WIDTH = 240;
// Icon center sits at nav-pl(12) + btn-px(12) + icon/2(8) = 32px.
// Double that so the icon is horizontally centered when collapsed.
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
 * DOM during pointermove. A 3px dead zone distinguishes clicks from
 * drags — clicks toggle, drags commit based on direction.
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
			const startX = e.clientX;

			// Movement under 3px counts as a click, not a drag.
			// Prevents accidental toggles from tiny jitters.
			const CLICK_DEAD_ZONE = 3;
			let dragging = false;

			// Kill the CSS transition so DOM writes are instant.
			container.style.transition = "none";

			const handlePointerMove = (moveEvent: PointerEvent) => {
				const dx = Math.abs(moveEvent.clientX - startX);
				if (!dragging && dx >= CLICK_DEAD_ZONE) {
					dragging = true;
				}
				if (dragging) {
					const rawWidth = moveEvent.clientX - startLeft;
					const clamped = Math.max(
						COLLAPSED_WIDTH,
						Math.min(rawWidth, EXPANDED_WIDTH),
					);
					container.style.width = `${clamped}px`;
				}
			};

			const cleanup = () => {
				document.removeEventListener("pointermove", handlePointerMove);
				document.removeEventListener("pointerup", cleanup);
				document.body.style.cursor = "";
				document.body.style.userSelect = "";

				let shouldCollapse: boolean;
				if (!dragging) {
					// Click (or movement < 3px) — toggle.
					shouldCollapse = !collapsed;
				} else {
					// Any drag in the correct direction completes
					// the transition.
					const finalWidth = container.getBoundingClientRect().width;
					shouldCollapse = finalWidth < startWidth;
				}
				const snapWidth = shouldCollapse ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

				// Re-enable transition for the snap animation.
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
