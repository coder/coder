import { useCallback, useState } from "react";

const EXPANDED_WIDTH = 240;
// Icon center sits at nav-pl(12) + btn-px(12) + icon/2(8) = 32px.
// Double that so the icon is horizontally centered when collapsed.
const COLLAPSED_WIDTH = 64;
const SNAP_THRESHOLD = 148;

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
	/** Force the sidebar to expand (used when navigating via collapsed icons). */
	expand: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

/**
 * Two-state sidebar: either EXPANDED_WIDTH or COLLAPSED_WIDTH.
 * Dragging across the snap threshold toggles between the two.
 * No in-between widths — the sidebar is always one or the other.
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
): UseSidebarResizeReturn {
	const [collapsed, setCollapsed] = useState(() => readCollapsed(storageKey));

	const expand = useCallback(() => {
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey]);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();

			// The handle sits on the outer wrapper div, not inside
			// the nav. Walk up to find the positioned container.
			const container =
				(e.currentTarget as HTMLElement).closest("[data-sidebar-container]") ??
				(e.currentTarget as HTMLElement).parentElement;
			const startLeft = container?.getBoundingClientRect().left ?? 0;

			const handlePointerMove = (moveEvent: PointerEvent) => {
				const rawWidth = moveEvent.clientX - startLeft;
				const shouldCollapse = rawWidth < SNAP_THRESHOLD;

				setCollapsed((prev) => {
					if (prev !== shouldCollapse) {
						persistCollapsed(storageKey, shouldCollapse);
					}
					return shouldCollapse;
				});
			};

			const cleanup = () => {
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
		[storageKey],
	);

	const width = collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

	return { width, collapsed, expand, onDragStart };
}
