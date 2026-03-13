import {
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	type RefObject,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

const STORAGE_KEY = "agents.right-panel-width";
const MIN_WIDTH = 360;
const MAX_WIDTH_RATIO = 0.7;
const DEFAULT_WIDTH = 480;

const SNAP_THRESHOLD = 80;

function getMaxWidth(): number {
	if (typeof window === "undefined") {
		return 960;
	}
	return Math.max(MIN_WIDTH, Math.floor(window.innerWidth * MAX_WIDTH_RATIO));
}

function loadPersistedWidth(): number {
	if (typeof window === "undefined") {
		return DEFAULT_WIDTH;
	}
	const stored = localStorage.getItem(STORAGE_KEY);
	if (!stored) {
		return DEFAULT_WIDTH;
	}
	const parsed = Number.parseInt(stored, 10);
	if (Number.isNaN(parsed) || parsed < MIN_WIDTH || parsed > getMaxWidth()) {
		return DEFAULT_WIDTH;
	}
	return parsed;
}

interface RightPanelProps {
	isOpen: boolean;
	isExpanded: boolean;
	onToggleExpanded: () => void;
	onClose: () => void;
	/** Fires during drag with the live visual expanded state, and
	 * null when the drag ends so the parent falls back to the
	 * committed isExpanded prop. */
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
	isSidebarCollapsed?: boolean;
	onToggleSidebarCollapsed?: () => void;
	children: ReactNode;
}

/**
 * Encapsulates all drag/resize logic for the right panel:
 * refs, pointer handlers, snap state, and visual state
 * derivation.
 */

function useResizableDrag({
	isExpanded,
	width,
	setWidth,
	panelRef,
	contentRef,
	isOpen,
	onSnapCommit,
	onVisualExpandedChange,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}: {
	isExpanded: boolean;
	width: number;
	setWidth: React.Dispatch<React.SetStateAction<number>>;
	panelRef: RefObject<HTMLDivElement | null>;
	contentRef: RefObject<HTMLDivElement | null>;
	isOpen: boolean;
	onSnapCommit: (snap: "normal" | "expanded" | "closed") => void;
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
	isSidebarCollapsed?: boolean;
	onToggleSidebarCollapsed?: () => void;
}) {
	const isDragging = useRef(false);
	const startX = useRef(0);
	const startWidth = useRef(0);
	const sidebarCollapsedByDrag = useRef(false);

	// Live width tracked via ref during drag so we can update the
	// DOM directly without triggering React re-renders on every
	// pointer-move. Committed to React state on pointer-up.
	const liveWidthRef = useRef(width);
	liveWidthRef.current = width;

	// Track snap state during a drag. This is state (not a ref) so
	// the panel visually updates as the user drags across thresholds.
	const [dragSnap, setDragSnap] = useState<
		"normal" | "expanded" | "closed" | null
	>(null);
	// Mirror of dragSnap used to avoid redundant setState calls
	// (same string) and to read the latest snap in handlePointerUp
	// without a stale closure.
	const dragSnapRef = useRef<"normal" | "expanded" | "closed" | null>(null);

	// Pending animation frame id so we can cancel stale frames.
	const rafId = useRef<number | null>(null);

	// Track the last visual-expanded value we reported so we only
	// notify the parent when the value actually changes.
	const lastVisualExpanded = useRef<boolean | undefined>(undefined);

	const handlePointerDown = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			e.preventDefault();
			isDragging.current = true;
			sidebarCollapsedByDrag.current = false;
			lastVisualExpanded.current = false;

			// Pre-set the snap zone to "normal" so the first
			// pointer-move doesn't trigger a wasted React
			// re-render (the derived visualExpanded and
			// visualOpen values are identical for null vs
			// "normal" when the panel is not expanded).
			dragSnapRef.current = "normal";
			if (isExpanded) {
				setDragSnap("normal");
			}

			startX.current = e.clientX;
			startWidth.current = isExpanded
				? ((e.target as HTMLElement).closest(
						"[data-testid='agents-right-panel']",
					)?.parentElement?.clientWidth ?? getMaxWidth())
				: liveWidthRef.current;
			(e.target as HTMLElement).setPointerCapture(e.pointerId);

			// Freeze content width so the expensive diff content doesn't
			// reflow on every pointer-move. The panel edge moves but the
			// inner content stays fixed. A single reflow happens on
			// pointer-up when we release the constraints.
			const content = contentRef.current;
			if (content) {
				const w = `${content.offsetWidth}px`;
				content.style.minWidth = w;
				content.style.maxWidth = w;
			}
			if (panelRef.current) {
				panelRef.current.style.overflow = "hidden";
			}
		},
		[isExpanded, panelRef, contentRef],
	);

	const handlePointerMove = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			if (!isDragging.current) {
				return;
			}
			const delta = startX.current - e.clientX;
			const raw = startWidth.current + delta;
			const maxWidth = getMaxWidth();

			// Collapse/uncollapse the sidebar live when the pointer
			// reaches the left edge of the viewport.
			if (e.clientX < SNAP_THRESHOLD && !sidebarCollapsedByDrag.current) {
				if (!isSidebarCollapsed && onToggleSidebarCollapsed) {
					onToggleSidebarCollapsed();
					sidebarCollapsedByDrag.current = true;
				}
			} else if (
				e.clientX >= SNAP_THRESHOLD &&
				sidebarCollapsedByDrag.current
			) {
				if (onToggleSidebarCollapsed) {
					onToggleSidebarCollapsed();
					sidebarCollapsedByDrag.current = false;
				}
			}

			let nextSnap: "normal" | "expanded" | "closed";
			let clampedWidth: number | null = null;
			if (raw > maxWidth + SNAP_THRESHOLD) {
				nextSnap = "expanded";
			} else if (raw < MIN_WIDTH - SNAP_THRESHOLD) {
				nextSnap = "closed";
			} else {
				nextSnap = "normal";
				clampedWidth = Math.min(maxWidth, Math.max(MIN_WIDTH, raw));
			}

			// Update the panel width directly on the DOM element so
			// we avoid a React re-render on every pointer-move. The
			// width is committed to React state once on pointer-up.
			if (clampedWidth !== null) {
				liveWidthRef.current = clampedWidth;
				panelRef.current?.style.setProperty(
					"--panel-width",
					`${clampedWidth}px`,
				);
			} // Only trigger React re-renders when the snap zone
			// changes (normal ↔ expanded ↔ closed). Within the
			// "normal" zone every pixel of movement is handled
			// purely via the DOM mutation above.
			if (nextSnap !== dragSnapRef.current) {
				dragSnapRef.current = nextSnap;
				setDragSnap(nextSnap);
			}

			// Notify parent of the live visual expanded state so
			// sibling content reacts during the drag, but only
			// when the value actually changes to avoid unnecessary
			// parent re-renders.
			const nextVisualExpanded =
				nextSnap === "expanded" ||
				(nextSnap !== "normal" && nextSnap !== "closed" && isExpanded);
			if (nextVisualExpanded !== lastVisualExpanded.current) {
				lastVisualExpanded.current = nextVisualExpanded;
				onVisualExpandedChange?.(nextVisualExpanded);
			}
		},
		[
			panelRef,
			isExpanded,
			onVisualExpandedChange,
			isSidebarCollapsed,
			onToggleSidebarCollapsed,
		],
	);

	const handlePointerUp = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			if (!isDragging.current) {
				return;
			}
			// Cancel any outstanding animation frame from the drag.
			if (rafId.current !== null) {
				cancelAnimationFrame(rafId.current);
				rafId.current = null;
			}
			const snap = dragSnapRef.current;
			isDragging.current = false;
			dragSnapRef.current = null;
			setDragSnap(null);
			(e.target as HTMLElement).releasePointerCapture(e.pointerId);

			// Unfreeze content so it reflows once to the final width.
			const content = contentRef.current;
			if (content) {
				content.style.minWidth = "";
				content.style.maxWidth = "";
			}
			if (panelRef.current) {
				panelRef.current.style.overflow = "";
			}

			// Commit the live width to React state so it persists
			// and is available for subsequent non-drag renders.
			setWidth(liveWidthRef.current);

			// Clear the drag override so parent falls back to its
			// own committed expanded state.
			onVisualExpandedChange?.(null);

			if (snap) {
				onSnapCommit(snap);
			}
		},
		[onSnapCommit, onVisualExpandedChange, setWidth, panelRef, contentRef],
	);

	// Derive visual state: during a drag the snap overrides the
	// committed parent state so the panel reacts live.
	const visualExpanded =
		dragSnap === "expanded" ||
		(dragSnap !== "normal" && dragSnap !== "closed" && isExpanded);
	const visualOpen =
		dragSnap !== "closed" &&
		(dragSnap === "expanded" || dragSnap === "normal" || isOpen);

	return {
		visualExpanded,
		visualOpen,
		handlePointerDown,
		handlePointerMove,
		handlePointerUp,
	};
}

export const RightPanel = ({
	isOpen,
	isExpanded,
	onToggleExpanded,
	onClose,
	onVisualExpandedChange,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	children,
}: RightPanelProps) => {
	const [width, setWidth] = useState(loadPersistedWidth);
	const panelRef = useRef<HTMLDivElement>(null);
	const contentRef = useRef<HTMLDivElement>(null);

	// Clamp width when the viewport shrinks so the panel
	// doesn't overflow and take over the whole page.
	useEffect(() => {
		const handleResize = () => {
			setWidth((prev) => Math.min(prev, getMaxWidth()));
		};
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, []);

	const handleSnapCommit = useCallback(
		(snap: "normal" | "expanded" | "closed") => {
			if (snap === "expanded" && !isExpanded) {
				onToggleExpanded();
			} else if (snap === "closed") {
				setWidth(DEFAULT_WIDTH);
				if (isExpanded) {
					onToggleExpanded();
				}
				onClose();
			} else if (snap === "normal" && isExpanded) {
				onToggleExpanded();
			}
		},
		[isExpanded, onToggleExpanded, onClose],
	);

	const {
		visualExpanded,
		visualOpen,
		handlePointerDown,
		handlePointerMove,
		handlePointerUp,
	} = useResizableDrag({
		isExpanded,
		width,
		setWidth,
		panelRef,
		contentRef,
		isOpen,
		onSnapCommit: handleSnapCommit,
		onVisualExpandedChange,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
	});

	useEffect(() => {
		if (typeof window !== "undefined") {
			localStorage.setItem(STORAGE_KEY, String(width));
		}
	}, [width]);

	return (
		<div
			ref={panelRef}
			data-testid="agents-right-panel"
			style={
				visualOpen && !visualExpanded
					? ({ "--panel-width": `${width}px` } as React.CSSProperties)
					: undefined
			}
			className={cn(
				visualExpanded
					? "absolute inset-0 z-30 flex flex-col"
					: cn(
							"relative min-h-0 min-w-0",
							visualOpen
								? "flex h-full w-[100vw] min-w-0 flex-col border-0 border-l border-solid border-border-default sm:w-[var(--panel-width)] sm:min-w-[360px] sm:max-w-[70vw]"
								: "hidden",
						),
			)}
		>
			{/* Drag handle (sm+, on the left edge of the panel) */}
			<div
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				className={cn(
					"absolute top-0 left-0 z-20 hidden h-full w-1 cursor-col-resize select-none transition-colors hover:bg-content-link sm:block",
					visualExpanded && "-left-1",
				)}
			/>
			<div ref={contentRef} className="flex min-h-0 flex-1 flex-col">
				{children}
			</div>
		</div>
	);
};
