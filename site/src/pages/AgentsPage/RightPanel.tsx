import {
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
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
	isOpen,
	onSnapCommit,
	onVisualExpandedChange,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
}: {
	isExpanded: boolean;
	width: number;
	setWidth: React.Dispatch<React.SetStateAction<number>>;
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
	// Track snap state during a drag. This is state (not a ref) so
	// the panel visually updates as the user drags across thresholds.
	const [dragSnap, setDragSnap] = useState<
		"normal" | "expanded" | "closed" | null
	>(null);

	const handlePointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
		e.preventDefault();
		isDragging.current = true;
		setDragSnap(null);
		sidebarCollapsedByDrag.current = false;
		startX.current = e.clientX;
		startWidth.current = isExpanded
			? ((e.target as HTMLElement).closest("[data-testid='agents-right-panel']")
					?.parentElement?.clientWidth ?? getMaxWidth())
			: width;
		(e.target as HTMLElement).setPointerCapture(e.pointerId);
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
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
		} else if (e.clientX >= SNAP_THRESHOLD && sidebarCollapsedByDrag.current) {
			if (onToggleSidebarCollapsed) {
				onToggleSidebarCollapsed();
				sidebarCollapsedByDrag.current = false;
			}
		}

		let nextSnap: "normal" | "expanded" | "closed";
		if (raw > maxWidth + SNAP_THRESHOLD) {
			nextSnap = "expanded";
		} else if (raw < MIN_WIDTH - SNAP_THRESHOLD) {
			nextSnap = "closed";
		} else {
			nextSnap = "normal";
			setWidth(Math.min(maxWidth, Math.max(MIN_WIDTH, raw)));
		}
		setDragSnap(nextSnap);

		// Notify parent of the live visual expanded state so
		// sibling content reacts during the drag.
		const nextVisualExpanded =
			nextSnap === "expanded" ||
			(nextSnap !== "normal" && nextSnap !== "closed" && isExpanded);
		onVisualExpandedChange?.(nextVisualExpanded);
	};

	const handlePointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}
		const snap = dragSnap;
		isDragging.current = false;
		setDragSnap(null);
		(e.target as HTMLElement).releasePointerCapture(e.pointerId);

		// Clear the drag override so parent falls back to its
		// own committed expanded state.
		onVisualExpandedChange?.(null);

		if (snap) {
			onSnapCommit(snap);
		}
	};

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

	// Clamp width when the viewport shrinks so the panel
	// doesn't overflow and take over the whole page.
	useEffect(() => {
		const handleResize = () => {
			setWidth((prev) => Math.min(prev, getMaxWidth()));
		};
		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, []);

	const handleSnapCommit = (snap: "normal" | "expanded" | "closed") => {
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
	};

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
			<div className="flex min-h-0 flex-1 flex-col">{children}</div>
		</div>
	);
};
