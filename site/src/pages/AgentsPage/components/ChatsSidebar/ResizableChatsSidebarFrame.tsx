import { cn } from "cn";
import {
	type KeyboardEvent as ReactKeyboardEvent,
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import {
	clampLeftSidebarWidth,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
	LEFT_SIDEBAR_MIN_WIDTH,
	loadPersistedLeftSidebarWidth,
	persistLeftSidebarWidth,
} from "./sidebarWidth";

interface ResizableChatsSidebarFrameProps {
	children: ReactNode;
	className?: string;
}

export const ResizableChatsSidebarFrame = ({
	children,
	className,
}: ResizableChatsSidebarFrameProps) => {
	const [width, setWidth] = useState(loadPersistedLeftSidebarWidth);
	const maxWidth = getLeftSidebarMaxWidth();
	const isDragging = useRef(false);
	const activePointerId = useRef<number | null>(null);
	const startX = useRef(0);
	const startWidth = useRef(0);

	const setVisualWidth = (nextWidth: number): number => {
		const clampedWidth = clampLeftSidebarWidth(nextWidth);
		setWidth(clampedWidth);
		return clampedWidth;
	};

	const setUserWidth = (nextWidth: number) => {
		const clampedWidth = setVisualWidth(nextWidth);
		persistLeftSidebarWidth(clampedWidth);
	};

	const handleResize = useEffectEvent(() => {
		const clampedWidth = clampLeftSidebarWidth(width);
		setVisualWidth(clampedWidth);
	});

	useEffect(() => {
		globalThis.addEventListener("resize", handleResize);
		return () => globalThis.removeEventListener("resize", handleResize);
	}, []);

	const handlePointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
		// Only a primary left-button pointer starts a drag; a second pointer
		// cannot take over one that is already in progress.
		if (isDragging.current || e.button !== 0 || !e.isPrimary) {
			return;
		}
		e.preventDefault();
		isDragging.current = true;
		activePointerId.current = e.pointerId;
		startX.current = e.clientX;
		startWidth.current = width;
		e.currentTarget.setPointerCapture?.(e.pointerId);
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current || e.pointerId !== activePointerId.current) {
			return;
		}

		const rawWidth = startWidth.current + (e.clientX - startX.current);
		setUserWidth(rawWidth);
	};

	// Ends the drag on pointerup, pointercancel, and lostpointercapture. The
	// last two fire without pointerup when the browser claims the gesture or
	// capture is lost (window deactivation, context menu), so all three must
	// reset the drag state.
	const handlePointerEnd = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current || e.pointerId !== activePointerId.current) {
			return;
		}

		isDragging.current = false;
		activePointerId.current = null;
		if (e.currentTarget.hasPointerCapture?.(e.pointerId)) {
			e.currentTarget.releasePointerCapture?.(e.pointerId);
		}
	};

	const handleKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
		switch (e.key) {
			case "ArrowLeft":
				e.preventDefault();
				setUserWidth(width - LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP);
				break;
			case "ArrowRight":
				e.preventDefault();
				setUserWidth(width + LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP);
				break;
			case "Home":
				e.preventDefault();
				setUserWidth(LEFT_SIDEBAR_MIN_WIDTH);
				break;
			case "End":
				e.preventDefault();
				setUserWidth(getLeftSidebarMaxWidth());
				break;
		}
	};

	return (
		<div
			data-testid="agents-sidebar-panel"
			style={{
				"--agents-left-sidebar-width": `${width}px`,
				"--agents-left-sidebar-min-width": `${LEFT_SIDEBAR_MIN_WIDTH}px`,
				"--agents-left-sidebar-max-width": `${maxWidth}px`,
			}}
			className={cn(
				className,
				"relative sm:w-(--agents-left-sidebar-width) sm:min-w-(--agents-left-sidebar-min-width) sm:max-w-(--agents-left-sidebar-max-width)",
			)}
		>
			{children}
			<div
				role="separator"
				aria-orientation="vertical"
				aria-label="Resize agents sidebar"
				aria-valuemin={LEFT_SIDEBAR_MIN_WIDTH}
				aria-valuemax={maxWidth}
				aria-valuenow={width}
				tabIndex={0}
				data-testid="agents-sidebar-resize-handle"
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerEnd}
				onPointerCancel={handlePointerEnd}
				onLostPointerCapture={handlePointerEnd}
				onKeyDown={handleKeyDown}
				className="absolute top-0 right-0 z-20 hidden h-full w-1 touch-none cursor-col-resize select-none transition-colors hover:bg-content-link focus-visible:bg-content-link focus-visible:outline-hidden sm:block"
			/>
		</div>
	);
};
