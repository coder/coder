import { useCallback, useEffect, useRef, useState } from "react";
import { useIsBelowLgViewport } from "#/hooks/useIsBelowLgViewport";
import { isBelowLgViewport } from "#/utils/mobile";

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
	/** Toggle collapsed/expanded state. */
	toggle: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

/**
 * Two-state sidebar that drags smoothly by writing directly to the
 * DOM during pointermove. A 3px dead zone distinguishes clicks from
 * drags. Clicks toggle via React state (CSS transition animates),
 * drags manipulate the DOM directly then snap on release.
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
): UseSidebarResizeReturn {
	// Start collapsed on narrow viewports regardless of the persisted
	// preference, so page content is not cut off on load.
	const [collapsed, setCollapsed] = useState(
		() => isBelowLgViewport() || readCollapsed(storageKey),
	);
	const isNarrowViewport = useIsBelowLgViewport();

	// Auto-collapse when the viewport shrinks below the lg breakpoint
	// and restore the persisted preference when it grows back. Forced
	// collapses are environmental, not user choices, so they are never
	// persisted. The ref limits this to actual crossings; the initial
	// narrow state is handled by the state initializer above.
	const prevNarrowRef = useRef(isNarrowViewport);
	useEffect(() => {
		if (prevNarrowRef.current === isNarrowViewport) {
			return;
		}
		prevNarrowRef.current = isNarrowViewport;
		setCollapsed(isNarrowViewport ? true : readCollapsed(storageKey));
	}, [isNarrowViewport, storageKey]);

	const expand = useCallback(() => {
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey]);

	const toggle = useCallback(() => {
		setCollapsed((prev) => {
			const next = !prev;
			persistCollapsed(storageKey, next);
			return next;
		});
	}, [storageKey]);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();

			const container =
				(e.currentTarget as HTMLElement).closest<HTMLElement>(
					"[data-sidebar-container]",
				) ?? (e.currentTarget as HTMLElement).parentElement;
			if (!container) {
				return () => {};
			}

			const startLeft = container.getBoundingClientRect().left;
			const startWidth = container.getBoundingClientRect().width;
			const startX = e.clientX;

			const CLICK_DEAD_ZONE = 3;
			let dragging = false;

			const handlePointerMove = (moveEvent: PointerEvent) => {
				const dx = Math.abs(moveEvent.clientX - startX);

				if (!dragging && dx >= CLICK_DEAD_ZONE) {
					dragging = true;
					// Only kill the transition once we know it's a real
					// drag, not a click. This keeps the CSS transition
					// intact for click-to-toggle.
					container.style.transition = "none";
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

				if (!dragging) {
					// Click: toggle via React state. The existing CSS
					// transition on the container animates the change.
					const next = !collapsed;
					setCollapsed(next);
					persistCollapsed(storageKey, next);
				} else {
					// Drag: snap based on direction.
					const finalWidth = container.getBoundingClientRect().width;
					const shouldCollapse = finalWidth < startWidth;
					const snapWidth = shouldCollapse ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

					// Re-enable transition for the snap animation.
					container.style.transition = "";
					container.style.width = `${snapWidth}px`;

					setCollapsed(shouldCollapse);
					persistCollapsed(storageKey, shouldCollapse);
				}
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

	return { width, collapsed, expand, toggle, onDragStart };
}
