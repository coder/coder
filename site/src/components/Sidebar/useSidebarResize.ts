import { useCallback, useRef, useState } from "react";

const EXPANDED_WIDTH = 240;
// Icon center sits at nav-pl(12) + btn-px(12) + icon/2(8) = 32px.
// Double that so the icon is horizontally centered when collapsed.
const COLLAPSED_WIDTH = 64;
const SNAP_THRESHOLD = 148;
// How many pixels of real cursor-tracking before snapping to the
// target width. Gives immediate visual feedback on grab.
const LEAD_IN_PX = 15;

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
	dragging: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

/**
 * Two-state sidebar with a smooth lead-in on drag. The first ~15px
 * of cursor movement tracks 1:1 so the grab feels immediate. After
 * that, the width snaps to the target (expanded or collapsed).
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
): UseSidebarResizeReturn {
	const [collapsed, setCollapsed] = useState(() => readCollapsed(storageKey));
	const [dragWidth, setDragWidth] = useState<number | null>(null);
	const startWidth = useRef(0);

	const expand = useCallback(() => {
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey]);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();

			const container =
				(e.currentTarget as HTMLElement).closest("[data-sidebar-container]") ??
				(e.currentTarget as HTMLElement).parentElement;
			const startLeft = container?.getBoundingClientRect().left ?? 0;
			const currentWidth = collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH;
			startWidth.current = currentWidth;

			// Begin tracking real pixel width immediately.
			setDragWidth(currentWidth);

			const handlePointerMove = (moveEvent: PointerEvent) => {
				const rawWidth = moveEvent.clientX - startLeft;
				const delta = rawWidth - startWidth.current;
				const absDelta = Math.abs(delta);

				if (absDelta <= LEAD_IN_PX) {
					// Lead-in: track the cursor 1:1 for tactile feedback.
					setDragWidth(
						Math.max(COLLAPSED_WIDTH, Math.min(rawWidth, EXPANDED_WIDTH)),
					);
				} else {
					// Past the lead-in: snap to the target width.
					const shouldCollapse = rawWidth < SNAP_THRESHOLD;
					const targetWidth = shouldCollapse ? COLLAPSED_WIDTH : EXPANDED_WIDTH;
					setDragWidth(targetWidth);

					setCollapsed((prev) => {
						if (prev !== shouldCollapse) {
							persistCollapsed(storageKey, shouldCollapse);
						}
						return shouldCollapse;
					});
				}
			};

			const cleanup = () => {
				// Snap to final resting width and clear drag state.
				setDragWidth(null);
				document.removeEventListener("pointermove", handlePointerMove);
				document.removeEventListener("pointerup", cleanup);
				document.body.style.cursor = "";
				document.body.style.userSelect = "";
			};

			document.body.style.cursor = "col-resize";
			document.body.style.userSelect = "none";
			document.addEventListener("pointermove", handlePointerMove);
			document.addEventListener("pointerup", cleanup);

			return cleanup;
		},
		[collapsed, storageKey],
	);

	// During drag, use the live pixel width. Otherwise use the
	// snapped width based on collapsed state.
	const width = dragWidth ?? (collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH);
	const dragging = dragWidth !== null;

	return { width, collapsed, dragging, expand, onDragStart };
}
