import {
	type PointerEvent as ReactPointerEvent,
	type Ref,
	useCallback,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

const STORAGE_KEY = "agents.diff-panel-width";
const MIN_WIDTH = 360;
const MAX_WIDTH = 960;
const DEFAULT_WIDTH = 480;

function loadPersistedWidth(): number {
	if (typeof window === "undefined") {
		return DEFAULT_WIDTH;
	}
	const stored = localStorage.getItem(STORAGE_KEY);
	if (!stored) {
		return DEFAULT_WIDTH;
	}
	const parsed = Number.parseInt(stored, 10);
	if (Number.isNaN(parsed) || parsed < MIN_WIDTH || parsed > MAX_WIDTH) {
		return DEFAULT_WIDTH;
	}
	return parsed;
}

interface DiffRightPanelProps {
	ref?: Ref<HTMLDivElement>;
	isOpen: boolean;
}

/**
 * The right-side panel for the diff/files-changed view. Always mounted
 * so the portal ref is always available (fixes blank-on-reopen). When
 * closed the panel is hidden via CSS and takes no layout space. On xl+
 * screens the panel is horizontally resizable via a drag handle.
 */
export const DiffRightPanel = ({ ref, isOpen }: DiffRightPanelProps) => {
	const [width, setWidth] = useState(loadPersistedWidth);
	const isDragging = useRef(false);
	const startX = useRef(0);
	const startWidth = useRef(0);

	const handlePointerDown = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			e.preventDefault();
			isDragging.current = true;
			startX.current = e.clientX;
			startWidth.current = width;
			(e.target as HTMLElement).setPointerCapture(e.pointerId);
		},
		[width],
	);

	const handlePointerMove = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			if (!isDragging.current) {
				return;
			}
			// Dragging left (negative delta) should make the panel wider
			// since the handle is on the left edge.
			const delta = startX.current - e.clientX;
			const next = Math.min(
				MAX_WIDTH,
				Math.max(MIN_WIDTH, startWidth.current + delta),
			);
			setWidth(next);
		},
		[],
	);

	const handlePointerUp = useCallback(
		(e: ReactPointerEvent<HTMLDivElement>) => {
			if (!isDragging.current) {
				return;
			}
			isDragging.current = false;
			(e.target as HTMLElement).releasePointerCapture(e.pointerId);
		},
		[],
	);

	// Persist width to localStorage when dragging ends.
	useEffect(() => {
		if (typeof window !== "undefined") {
			localStorage.setItem(STORAGE_KEY, String(width));
		}
	}, [width]);

	return (
		<div
			ref={ref}
			data-testid="agents-detail-right-panel"
			style={
				isOpen
					? ({ "--panel-width": `${width}px` } as React.CSSProperties)
					: undefined
			}
			className={cn(
				"relative min-h-0 min-w-0 border-t border-border-default bg-surface-primary",
				isOpen
					? "h-[42dvh] min-h-[260px] max-h-[56dvh] xl:h-auto xl:max-h-none xl:w-[var(--panel-width)] xl:min-w-[360px] xl:max-w-[960px] xl:border-l xl:border-t-0"
					: "hidden",
			)}
		>
			{/* Drag handle (xl+ only, on the left edge of the panel) */}
			<div
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				className="absolute top-0 left-0 z-10 hidden h-full w-1 cursor-col-resize select-none transition-colors hover:bg-content-link xl:block"
			/>
		</div>
	);
};
