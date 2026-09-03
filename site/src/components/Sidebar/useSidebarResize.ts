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

interface UseSidebarResizeOptions {
	/**
	 * The current page has wide content and the sidebar should settle
	 * collapsed. Environmental like the narrow-viewport rule: never
	 * persisted, and the stored preference is restored when it clears.
	 */
	preferCollapsed?: boolean;
}

interface UseSidebarResizeReturn {
	width: number;
	collapsed: boolean;
	/**
	 * The sidebar just settled collapsed for a wide page and is briefly
	 * showing the expanded nav as a flyout over the content.
	 */
	peeking: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	/** Toggle collapsed/expanded state. */
	toggle: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

const PEEK_DURATION_MS = 1500;

/**
 * Two-state sidebar that drags smoothly by writing directly to the
 * DOM during pointermove. A 3px dead zone distinguishes clicks from
 * drags. Clicks toggle via React state (CSS transition animates),
 * drags manipulate the DOM directly then snap on release.
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
	{ preferCollapsed = false }: UseSidebarResizeOptions = {},
): UseSidebarResizeReturn {
	// Start collapsed on narrow viewports and wide-content pages
	// regardless of the persisted preference, so page content is not
	// cut off on load.
	const [collapsed, setCollapsed] = useState(
		() => isBelowLgViewport() || preferCollapsed || readCollapsed(storageKey),
	);
	const [peeking, setPeeking] = useState(false);
	const peekTimerRef = useRef<number | undefined>(undefined);
	const isNarrowViewport = useIsBelowLgViewport();

	const endPeek = useCallback(() => {
		if (peekTimerRef.current !== undefined) {
			window.clearTimeout(peekTimerRef.current);
			peekTimerRef.current = undefined;
		}
		setPeeking(false);
	}, []);

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
		if (isNarrowViewport) {
			endPeek();
			setCollapsed(true);
		} else {
			setCollapsed(preferCollapsed || readCollapsed(storageKey));
		}
	}, [isNarrowViewport, preferCollapsed, storageKey, endPeek]);

	// Entering the wide-content set collapses the sidebar in layout flow
	// and, when it was expanded, peeks the nav as a flyout so the user
	// sees where it went. Leaving the set restores the preference.
	// Moving between two wide pages changes nothing.
	const prevPreferRef = useRef<boolean | undefined>(undefined);
	useEffect(() => {
		const isMount = prevPreferRef.current === undefined;
		if (prevPreferRef.current === preferCollapsed) {
			return;
		}
		prevPreferRef.current = preferCollapsed;

		if (!preferCollapsed) {
			if (!isMount) {
				endPeek();
				setCollapsed(isBelowLgViewport() || readCollapsed(storageKey));
			}
			return;
		}

		if (isBelowLgViewport()) {
			return;
		}
		// On mount the initializer already collapsed the sidebar, so the
		// stored preference tells us whether it would have been expanded.
		const wasExpanded = isMount ? !readCollapsed(storageKey) : !collapsed;
		if (!wasExpanded) {
			return;
		}
		setCollapsed(true);
		setPeeking(true);
		peekTimerRef.current = window.setTimeout(() => {
			peekTimerRef.current = undefined;
			setPeeking(false);
		}, PEEK_DURATION_MS);
	}, [preferCollapsed, collapsed, storageKey, endPeek]);

	// Any interaction dismisses the peek early.
	useEffect(() => {
		if (!peeking) {
			return;
		}
		document.addEventListener("pointerdown", endPeek);
		document.addEventListener("keydown", endPeek);
		return () => {
			document.removeEventListener("pointerdown", endPeek);
			document.removeEventListener("keydown", endPeek);
		};
	}, [peeking, endPeek]);

	const expand = useCallback(() => {
		endPeek();
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey, endPeek]);

	const toggle = useCallback(() => {
		endPeek();
		setCollapsed((prev) => {
			const next = !prev;
			persistCollapsed(storageKey, next);
			return next;
		});
	}, [storageKey, endPeek]);

	const onDragStart = useCallback(
		(e: React.PointerEvent): (() => void) => {
			e.preventDefault();
			endPeek();

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
		[collapsed, storageKey, endPeek],
	);

	const width = collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH;

	return { width, collapsed, peeking, expand, toggle, onDragStart };
}
