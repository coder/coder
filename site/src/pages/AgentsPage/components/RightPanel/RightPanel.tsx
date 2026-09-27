import { cn } from "cn";
import {
	type AnimationEvent as ReactAnimationEvent,
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	type TransitionEvent as ReactTransitionEvent,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { useOutletContext } from "react-router";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { AGENTS_MAIN_PANEL_MIN_WIDTH } from "../ChatsSidebar/sidebarWidth";

export const RIGHT_PANEL_OPEN_KEY = "agents.right-panel-open";
export const RIGHT_PANEL_WIDTH_KEY = "agents.right-panel-width";

const MIN_WIDTH = 360;
const MAX_WIDTH_RATIO = 0.7;
const DEFAULT_WIDTH = 480;

const SNAP_THRESHOLD = 80;
const RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH = 1024;
// Dead band between the collapse and restore thresholds, so a window near
// the threshold does not collapse and restore the sidebar on every resize.
const SIDEBAR_RESTORE_HYSTERESIS = 24;

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
	const stored = localStorage.getItem(RIGHT_PANEL_WIDTH_KEY);
	if (!stored) {
		return DEFAULT_WIDTH;
	}
	const parsed = Number.parseInt(stored, 10);
	if (Number.isNaN(parsed) || parsed < MIN_WIDTH || parsed > getMaxWidth()) {
		return DEFAULT_WIDTH;
	}
	return parsed;
}

type RightPanelProps = {
	isOpen: boolean;
	isExpanded: boolean;
	onToggleExpanded: () => void;
	onClose: () => void;
	/** Fires during drag with the live visual expanded state, and
	 * null when the drag ends so the parent falls back to the
	 * committed isExpanded prop. */
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
	children: ReactNode;
};

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
	const activePointerId = useRef<number | null>(null);
	const startX = useRef(0);
	const startWidth = useRef(0);
	const sidebarCollapsedByDrag = useRef(false);
	// Track snap state during a drag. This is state (not a ref) so
	// the panel visually updates as the user drags across thresholds.
	// The ref mirrors it for the terminal handlers: a pointerup can
	// arrive before the state update from the last pointermove has
	// rendered, and the commit must use the zone the pointer ended in.
	const [dragSnap, setDragSnap] = useState<
		"normal" | "expanded" | "closed" | null
	>(null);
	const snapRef = useRef<"normal" | "expanded" | "closed" | null>(null);

	const handlePointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (isDragging.current || e.button !== 0 || !e.isPrimary) {
			return;
		}
		e.preventDefault();
		isDragging.current = true;
		activePointerId.current = e.pointerId;
		snapRef.current = null;
		setDragSnap(null);
		sidebarCollapsedByDrag.current = false;
		startX.current = e.clientX;
		const panel = e.currentTarget.closest("[data-testid='agents-right-panel']");
		startWidth.current = panel?.getBoundingClientRect().width ?? width;
		e.currentTarget.setPointerCapture(e.pointerId);
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current || e.pointerId !== activePointerId.current) {
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
		snapRef.current = nextSnap;
		setDragSnap(nextSnap);

		// Notify parent of the live visual expanded state so
		// sibling content reacts during the drag.
		const nextVisualExpanded =
			nextSnap === "expanded" ||
			(nextSnap !== "normal" && nextSnap !== "closed" && isExpanded);
		onVisualExpandedChange?.(nextVisualExpanded);
	};

	// Ends the drag for the active pointer. A pointerup commits the snap
	// the pointer ended in; pointercancel and lostpointercapture clear the
	// drag override without committing and keep the live width. A normal
	// release also fires lostpointercapture, which the isDragging guard
	// turns into a no-op.
	const finishDrag = (
		e: ReactPointerEvent<HTMLDivElement>,
		{ commit }: { commit: boolean },
	) => {
		if (!isDragging.current || e.pointerId !== activePointerId.current) {
			return;
		}
		const snap = snapRef.current;
		isDragging.current = false;
		activePointerId.current = null;
		snapRef.current = null;
		setDragSnap(null);
		if (e.currentTarget.hasPointerCapture(e.pointerId)) {
			e.currentTarget.releasePointerCapture(e.pointerId);
		}

		// Clear the drag override so parent falls back to its
		// own committed expanded state.
		onVisualExpandedChange?.(null);

		if (!commit) {
			if (
				sidebarCollapsedByDrag.current &&
				isSidebarCollapsed &&
				onToggleSidebarCollapsed
			) {
				onToggleSidebarCollapsed();
			}
			sidebarCollapsedByDrag.current = false;
			return;
		}
		if (snap) {
			onSnapCommit(snap);
		}
	};

	const handlePointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
		finishDrag(e, { commit: true });
	};

	const handlePointerAbort = (e: ReactPointerEvent<HTMLDivElement>) => {
		finishDrag(e, { commit: false });
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
		isPointerResizing: dragSnap !== null,
		handlePointerDown,
		handlePointerMove,
		handlePointerUp,
		handlePointerAbort,
	};
}

export const RightPanel = ({
	isOpen,
	isExpanded,
	onToggleExpanded,
	onClose,
	onVisualExpandedChange,
	children,
}: RightPanelProps) => {
	const {
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		isSidebarCollapsedByNarrowWidth,
		onSidebarCollapsedByNarrowWidthChange,
		getExpandedSidebarWidth,
		registerOpenRightPanel,
	} = useOutletContext<AgentsPageOutletContext | undefined>() ?? {};
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
		isPointerResizing,
		handlePointerDown,
		handlePointerMove,
		handlePointerUp,
		handlePointerAbort,
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

	const isBelowLg = useMediaQuery(belowLgViewportMediaQuery);

	// Pin content width while opening so terminals don't refit every frame.
	// Below lg the panel opens as an overlay with no width transition.
	const [prevVisualOpen, setPrevVisualOpen] = useState(visualOpen);
	const [isAnimatingOpen, setIsAnimatingOpen] = useState(false);
	// Dropping below lg removes the lg: styles immediately, so a keyframe
	// animation slides the panel out from its last width.
	const [isNarrowSlideOut, setIsNarrowSlideOut] = useState(false);
	const isSideBySide = visualOpen && !visualExpanded && !isBelowLg;
	const [prevIsSideBySide, setPrevIsSideBySide] = useState(isSideBySide);
	if (visualOpen !== prevVisualOpen) {
		setPrevVisualOpen(visualOpen);
		setIsAnimatingOpen(visualOpen && !isPointerResizing && !isBelowLg);
		if (visualOpen) {
			// Reopening mid-slide cancels the animation without an animationend.
			setIsNarrowSlideOut(false);
		}
	} else if (isAnimatingOpen && isPointerResizing) {
		// A drag removes the transition, so no transitionend will unpin.
		setIsAnimatingOpen(false);
	}
	if (isSideBySide !== prevIsSideBySide) {
		setPrevIsSideBySide(isSideBySide);
		setIsNarrowSlideOut(!isSideBySide && isBelowLg && !visualExpanded);
	} else if (isNarrowSlideOut && !isBelowLg) {
		setIsNarrowSlideOut(false);
	}
	const handleWidthTransitionEnd = (e: ReactTransitionEvent) => {
		if (e.target === e.currentTarget && e.propertyName === "width") {
			setIsAnimatingOpen(false);
		}
	};
	const handleSlideOutEnd = (e: ReactAnimationEvent) => {
		if (e.target === e.currentTarget) {
			setIsNarrowSlideOut(false);
		}
	};
	const isContentPinned = !visualExpanded && (!visualOpen || isAnimatingOpen);

	useEffect(() => {
		localStorage.setItem(RIGHT_PANEL_WIDTH_KEY, String(width));
	}, [width]);

	// While open, keep a sidebar collapsed to fit this panel from being restored.
	useEffect(() => {
		if (!isOpen) {
			return;
		}
		return registerOpenRightPanel?.();
	}, [isOpen, registerOpenRightPanel]);

	const getPanelWidth = useEffectEvent(() => width);

	useEffect(() => {
		if (
			!visualOpen ||
			visualExpanded ||
			isBelowLg ||
			!onSidebarCollapsedByNarrowWidthChange ||
			(isSidebarCollapsed && !isSidebarCollapsedByNarrowWidth)
		) {
			return;
		}

		const parent = panelRef.current?.parentElement;
		if (!parent) {
			return;
		}

		let frame = 0;
		let narrowWidthChangeRequested = false;
		const maybeUpdateNarrowWidthCollapse = () => {
			cancelAnimationFrame(frame);
			frame = requestAnimationFrame(() => {
				if (narrowWidthChangeRequested) {
					return;
				}

				const chatMinWidth = getChatMinWidth(parent);
				if (isSidebarCollapsed) {
					const sidebarWidth = getExpandedSidebarWidth?.();
					if (sidebarWidth === undefined) {
						return;
					}
					// Leave room for the panel's current width, not just its minimum.
					const requiredWidth =
						chatMinWidth +
						Math.max(MIN_WIDTH, getPanelWidth()) +
						SIDEBAR_RESTORE_HYSTERESIS;
					if (parent.clientWidth - sidebarWidth < requiredWidth) {
						return;
					}
				} else if (parent.clientWidth >= chatMinWidth + MIN_WIDTH) {
					return;
				}

				narrowWidthChangeRequested = true;
				onSidebarCollapsedByNarrowWidthChange(!isSidebarCollapsed);
			});
		};

		maybeUpdateNarrowWidthCollapse();
		const resizeObserver = new ResizeObserver(maybeUpdateNarrowWidthCollapse);
		resizeObserver.observe(parent);
		addEventListener("resize", maybeUpdateNarrowWidthCollapse);

		return () => {
			cancelAnimationFrame(frame);
			resizeObserver.disconnect();
			removeEventListener("resize", maybeUpdateNarrowWidthCollapse);
		};
	}, [
		visualOpen,
		visualExpanded,
		isBelowLg,
		isSidebarCollapsed,
		isSidebarCollapsedByNarrowWidth,
		onSidebarCollapsedByNarrowWidthChange,
		getExpandedSidebarWidth,
	]);

	return (
		<div
			ref={panelRef}
			data-testid="agents-right-panel"
			style={visualExpanded ? undefined : { "--panel-width": `${width}px` }}
			onTransitionEnd={handleWidthTransitionEnd}
			onAnimationEnd={handleSlideOutEnd}
			className={cn(
				!visualExpanded &&
					!isPointerResizing &&
					"lg:transition-[width,visibility] lg:duration-(--panel-slide-duration) lg:ease-out",
				visualExpanded
					? "absolute inset-0 z-30 flex flex-col"
					: visualOpen
						? "fixed inset-0 z-30 flex flex-col bg-surface-primary lg:relative lg:inset-auto lg:z-auto lg:h-full lg:min-h-0 lg:min-w-0 lg:items-end lg:overflow-hidden lg:border-0 lg:border-l lg:border-solid lg:border-border-default lg:w-[min(var(--panel-width),max(0px,calc(100%-var(--agents-chat-panel-min-width,0px))))] lg:max-w-[70vw]"
						: isNarrowSlideOut
							? "relative invisible flex h-full min-h-0 w-0 min-w-0 flex-col items-end overflow-hidden border-0 border-l border-solid border-border-default animate-panel-slide-out"
							: "relative min-h-0 min-w-0 hidden lg:invisible lg:flex lg:h-full lg:w-0 lg:flex-col lg:items-end lg:overflow-hidden",
			)}
		>
			{/* Drag handle (sm+, on the left edge of the panel) */}
			<div
				data-testid="agents-right-panel-resize-handle"
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				onPointerCancel={handlePointerAbort}
				onLostPointerCapture={handlePointerAbort}
				className={cn(
					"absolute top-0 left-0 z-20 hidden h-full w-1 touch-none cursor-col-resize select-none transition-colors hover:bg-content-link lg:block",
					visualExpanded && "-left-1",
				)}
			/>
			<div
				className={cn(
					"flex min-h-0 w-full flex-1 flex-col",
					isContentPinned &&
						(isNarrowSlideOut ? "w-(--panel-width)" : "lg:w-(--panel-width)"),
				)}
			>
				{children}
			</div>
		</div>
	);
};
