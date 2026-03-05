import { Button } from "components/Button/Button";
import { MaximizeIcon, MinimizeIcon, PanelLeftIcon } from "lucide-react";
import {
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
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

const TABS = [{ id: "git", label: "Git" }];

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
	chatTitle?: string;
	isSidebarCollapsed?: boolean;
	onToggleSidebarCollapsed?: () => void;
	tabContent: Record<string, ReactNode>;
	/** Fires during drag with the live visual expanded state, and
	 * null when the drag ends so the parent falls back to the
	 * committed isExpanded prop. */
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
}

/**
 * Encapsulates all drag/resize logic for the right panel:
 * refs, pointer handlers, snap state, sidebar collapse
 * tracking, and visual state derivation.
 */
function useResizableDrag({
	isExpanded,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	width,
	setWidth,
	isOpen,
	onSnapCommit,
	onVisualExpandedChange,
}: {
	isExpanded: boolean;
	isSidebarCollapsed?: boolean;
	onToggleSidebarCollapsed?: () => void;
	width: number;
	setWidth: React.Dispatch<React.SetStateAction<number>>;
	isOpen: boolean;
	onSnapCommit: (snap: "normal" | "expanded" | "closed") => void;
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
}) {
	const isDragging = useRef(false);
	const startX = useRef(0);
	const startWidth = useRef(0);
	// Track snap state during a drag. This is state (not a ref) so
	// the panel visually updates as the user drags across thresholds.
	const [dragSnap, setDragSnap] = useState<
		"normal" | "expanded" | "closed" | null
	>(null);
	// Whether we collapsed the sidebar during this drag gesture.
	// Used to reverse it if the user drags back.
	const sidebarCollapsedByDrag = useRef(false);

	const handlePointerDown = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			e.preventDefault();
			isDragging.current = true;
			sidebarCollapsedByDrag.current = false;
			setDragSnap(null);
			startX.current = e.clientX;
			startWidth.current = isExpanded
				? ((e.target as HTMLElement).closest(
						"[data-testid='agents-right-panel']",
					)?.parentElement?.clientWidth ?? getMaxWidth())
				: width;
			(e.target as HTMLElement).setPointerCapture(e.pointerId);
		},
		[width, isExpanded],
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
		},
		[
			isSidebarCollapsed,
			onToggleSidebarCollapsed,
			setWidth,
			isExpanded,
			onVisualExpandedChange,
		],
	);

	const handlePointerUp = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
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
		},
		[dragSnap, onSnapCommit, onVisualExpandedChange],
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
		sidebarCollapsedByDrag,
	};
}

export const RightPanel = ({
	isOpen,
	isExpanded,
	onToggleExpanded,
	onClose,
	chatTitle,
	isSidebarCollapsed,
	onToggleSidebarCollapsed,
	tabContent,
	onVisualExpandedChange,
}: RightPanelProps) => {
	const [activeTab, setActiveTab] = useState("git");
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
		isSidebarCollapsed,
		onToggleSidebarCollapsed,
		width,
		setWidth,
		isOpen,
		onSnapCommit: handleSnapCommit,
		onVisualExpandedChange,
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
								? "flex h-[42dvh] min-h-[260px] max-h-[56dvh] flex-col xl:h-auto xl:max-h-none xl:w-[var(--panel-width)] xl:min-w-[360px] xl:max-w-[70vw] xl:border-0 xl:border-l xl:border-solid xl:border-border-default"
								: "hidden",
						),
			)}
		>
			{/* Drag handle (xl+ only, on the left edge of the panel) */}
			<div
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				className={cn(
					"absolute top-0 left-0 z-20 hidden h-full w-1 cursor-col-resize select-none transition-colors hover:bg-content-link xl:block",
					visualExpanded && "-left-1",
				)}
			/>{" "}
			<div className="flex min-h-0 flex-1 flex-col">
				{/* Tabbed header */}
				<div className="flex shrink-0 items-center gap-2 px-3 py-1">
					{/* Left side: sidebar toggle (expanded + collapsed only) + tabs */}
					<div className="flex items-center">
						{visualExpanded &&
							isSidebarCollapsed &&
							onToggleSidebarCollapsed && (
								<Button
									variant="subtle"
									size="icon"
									onClick={onToggleSidebarCollapsed}
									aria-label="Expand sidebar"
									className="mr-1 h-7 w-7 min-w-0 shrink-0"
								>
									<PanelLeftIcon />
								</Button>
							)}
						{TABS.map((tab) => (
							<Button
								key={tab.id}
								variant={activeTab === tab.id ? "outline" : "subtle"}
								size="lg"
								onClick={() => setActiveTab(tab.id)}
								className={cn(
									"min-w-0 h-6 px-3",
									activeTab === tab.id && "bg-surface-secondary",
								)}
							>
								{tab.label}
							</Button>
						))}
					</div>{" "}
					{/* Center: chat title */}
					<div className="min-w-0 flex-1 text-center">
						{visualExpanded && chatTitle && (
							<span className="truncate text-sm text-content-primary">
								{chatTitle}
							</span>
						)}
					</div>
					{/* Right side: expand/contract button */}
					<Button
						variant="subtle"
						size="icon"
						onClick={onToggleExpanded}
						aria-label={visualExpanded ? "Collapse panel" : "Expand panel"}
						className="h-7 w-7 text-content-secondary hover:text-content-primary"
					>
						{visualExpanded ? <MinimizeIcon /> : <MaximizeIcon />}
					</Button>
				</div>
				<div className="min-h-0 flex-1 overflow-hidden bg-surface-secondary/45">
					{tabContent[activeTab]}
				</div>
			</div>
		</div>
	);
};
