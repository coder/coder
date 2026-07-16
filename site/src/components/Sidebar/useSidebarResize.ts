import { useCallback, useEffect, useRef, useState } from "react";

const EXPANDED_WIDTH = 240;
// Icon center sits at nav-pl(12) + btn-px(12) + icon/2(8) = 32px.
// Double that so the icon is horizontally centered when collapsed.
const COLLAPSED_WIDTH = 64;

function readPersisted(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function readCollapsed(key: string): boolean {
	return readPersisted(key) === "collapsed";
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
	 * On mount, briefly expand the sidebar then collapse it after a
	 * short delay, unless the user previously left it expanded. The
	 * peek expansion is never persisted; any user interaction cancels
	 * the pending collapse.
	 */
	peekOnMount?: boolean;
}

interface UseSidebarResizeReturn {
	width: number;
	collapsed: boolean;
	/** Force the sidebar to expand. */
	expand: () => void;
	/** Force the sidebar to collapse. */
	collapse: () => void;
	/** Toggle collapsed/expanded state. */
	toggle: () => void;
	/** Cancel a pending peek-on-mount auto-collapse, if any. */
	cancelPeek: () => void;
	onDragStart: (e: React.PointerEvent) => () => void;
}

const PEEK_COLLAPSE_DELAY_MS = 1500;

/**
 * Two-state sidebar that drags smoothly by writing directly to the
 * DOM during pointermove. A 3px dead zone distinguishes clicks from
 * drags. Clicks toggle via React state (CSS transition animates),
 * drags manipulate the DOM directly then snap on release.
 */
export function useSidebarResize(
	storageKey = "sidebar-width",
	{ peekOnMount = false }: UseSidebarResizeOptions = {},
): UseSidebarResizeReturn {
	const [collapsed, setCollapsed] = useState(() => readCollapsed(storageKey));
	const peekTimerRef = useRef<number | undefined>(undefined);

	const cancelPeek = useCallback(() => {
		if (peekTimerRef.current !== undefined) {
			window.clearTimeout(peekTimerRef.current);
			peekTimerRef.current = undefined;
		}
	}, []);

	// Peek: expand on mount, then collapse after a delay. Skipped when
	// the user explicitly left the sidebar expanded. Neither the
	// expansion nor the auto-collapse is persisted.
	useEffect(() => {
		if (!peekOnMount || readPersisted(storageKey) === "expanded") {
			return;
		}
		setCollapsed(false);
		peekTimerRef.current = window.setTimeout(() => {
			peekTimerRef.current = undefined;
			setCollapsed(true);
		}, PEEK_COLLAPSE_DELAY_MS);
		return cancelPeek;
	}, [peekOnMount, storageKey, cancelPeek]);

	const expand = useCallback(() => {
		cancelPeek();
		setCollapsed(false);
		persistCollapsed(storageKey, false);
	}, [storageKey, cancelPeek]);

	const collapse = useCallback(() => {
		cancelPeek();
		setCollapsed(true);
		persistCollapsed(storageKey, true);
	}, [storageKey, cancelPeek]);

	const toggle = useCallback(() => {
		cancelPeek();
		setCollapsed((prev) => {
			const next = !prev;
			persistCollapsed(storageKey, next);
			return next;
		});
	}, [storageKey, cancelPeek]);

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

	return {
		width,
		collapsed,
		expand,
		collapse,
		toggle,
		cancelPeek,
		onDragStart,
	};
}
