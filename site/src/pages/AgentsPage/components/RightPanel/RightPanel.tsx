import {
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "#/utils/cn";
import { AGENTS_MAIN_PANEL_MIN_WIDTH } from "../ChatsSidebar/sidebarWidth";

const STORAGE_KEY = "agents.right-panel-width";
const MIN_WIDTH = 360;
const MAX_WIDTH_RATIO = 0.7;
const DEFAULT_WIDTH = 480;

const SNAP_THRESHOLD = 80;
const RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH = 1024;

function getMaxWidth(): number {
	return Math.max(MIN_WIDTH, Math.floor(window.innerWidth * MAX_WIDTH_RATIO));
}

function getChatMinWidth(parent: HTMLElement): number {
	const rawChatMinWidth = getComputedStyle(parent).getPropertyValue(
		"--agents-chat-panel-min-width",
	);
	const chatMinWidth = Number.parseFloat(rawChatMinWidth);
	return Number.isFinite(chatMinWidth) && chatMinWidth > 0
		? chatMinWidth
		: AGENTS_MAIN_PANEL_MIN_WIDTH;
}

function getSideBySideMaxWidth(panel: HTMLElement | null): number {
	const parent = panel?.parentElement;
	if (!parent || innerWidth < RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH) {
		return getMaxWidth();
	}

	return Math.min(
		getMaxWidth(),
		Math.max(MIN_WIDTH, parent.clientWidth - getChatMinWidth(parent)),
	);
}

function loadPersistedWidth(): number {
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
	getPanelMaxWidth,
}: {
	isExpanded: boolean;
	width: number;
	setWidth: React.Dispatch<React.SetStateAction<number>>;
	isOpen: boolean;
	onSnapCommit: (snap: "normal" | "expanded" | "closed") => void;
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
	isSidebarCollapsed?: boolean;
	onToggleSidebarCollapsed?: () => void;
	getPanelMaxWidth: () => number;
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
		const panel = (e.target as HTMLElement).closest(
			"[data-testid='agents-right-panel']",
		);
		startWidth.current = panel?.getBoundingClientRect().width ?? width;
		(e.target as HTMLElement).setPointerCapture(e.pointerId);
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}
		const delta = startX.current - e.clientX;
		const raw = startWidth.current + delta;
		const maxWidth = getPanelMaxWidth();

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
	const panelRef = useRef<HTMLDivElement>(null);

	// Clamp width when the viewport or parent panel shrinks so the
	// persisted width matches the rendered side-by-side panel width.
	useEffect(() => {
		const handleResize = () => {
			setWidth((prev) =>
				Math.min(prev, getSideBySideMaxWidth(panelRef.current)),
			);
		};
		handleResize();
		const parent = panelRef.current?.parentElement;
		const resizeObserver = new ResizeObserver(handleResize);
		if (parent) {
			resizeObserver.observe(parent);
		}
		window.addEventListener("resize", handleResize);
		return () => {
			resizeObserver.disconnect();
			window.removeEventListener("resize", handleResize);
		};
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
		getPanelMaxWidth: () => getSideBySideMaxWidth(panelRef.current),
	});

	useEffect(() => {
		localStorage.setItem(STORAGE_KEY, String(width));
	}, [width]);

	useEffect(() => {
		if (
			!visualOpen ||
			visualExpanded ||
			isSidebarCollapsed ||
			!onToggleSidebarCollapsed
		) {
			return;
		}

		const parent = panelRef.current?.parentElement;
		if (!parent) {
			return;
		}

		let frame = 0;
		let collapseRequested = false;
		const maybeCollapseSidebar = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(() => {
				if (
					collapseRequested ||
					innerWidth < RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH
				) {
					return;
				}

				const requiredMainWidth = getChatMinWidth(parent) + MIN_WIDTH;

				if (parent.clientWidth >= requiredMainWidth) {
					return;
				}

				collapseRequested = true;
				onToggleSidebarCollapsed();
			});
		};

		maybeCollapseSidebar();
		const resizeObserver = new ResizeObserver(maybeCollapseSidebar);
		resizeObserver.observe(parent);
		addEventListener("resize", maybeCollapseSidebar);

		return () => {
			cancelAnimationFrame(frame);
			resizeObserver.disconnect();
			removeEventListener("resize", maybeCollapseSidebar);
		};
	}, [
		visualOpen,
		visualExpanded,
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
	]);

	return (
		<div
			ref={panelRef}
			data-testid="agents-right-panel"
			style={
				visualOpen && !visualExpanded
					? { "--panel-width": `${width}px` }
					: undefined
			}
			className={cn(
				visualExpanded
					? "absolute inset-0 z-30 flex flex-col"
					: visualOpen
						? "fixed inset-0 z-30 flex flex-col bg-surface-primary lg:relative lg:inset-auto lg:z-auto lg:h-full lg:min-h-0 lg:min-w-0 lg:overflow-hidden lg:border-0 lg:border-l lg:border-solid lg:border-border-default lg:w-[min(var(--panel-width),max(0px,calc(100%_-_var(--agents-chat-panel-min-width,0px))))] lg:max-w-[70vw]"
						: "relative min-h-0 min-w-0 hidden",
			)}
		>
			{/* Drag handle (sm+, on the left edge of the panel) */}
			<div
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				className={cn(
					"absolute top-0 left-0 z-20 hidden h-full w-1 cursor-col-resize select-none transition-colors hover:bg-content-link lg:block",
					visualExpanded && "-left-1",
				)}
			/>
			<div className="flex min-h-0 flex-1 flex-col">{children}</div>
		</div>
	);
};
