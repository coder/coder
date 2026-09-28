import { cn } from "cn";
import { useEffect, useRef, useState } from "react";
import {
	clampLeftSidebarWidth,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
	LEFT_SIDEBAR_MIN_WIDTH,
	loadStoredLeftSidebarWidth,
	persistLeftSidebarWidth,
} from "./sidebarWidth";

type ResizableChatsSidebarFrameProps = {
	children: React.ReactNode;
	className?: string;
	isCollapsed?: boolean;
	/**
	 * Plays the sm-breakpoint slide. "out" ignores className and isCollapsed
	 * and needs a flex-row parent to shrink beside the main panel; "in" grows
	 * it from zero. The caller resets it to null from onViewportSlideEnd.
	 */
	viewportSlide?: "in" | "out" | null;
	onViewportSlideEnd?: () => void;
};

export const ResizableChatsSidebarFrame = ({
	children,
	className,
	isCollapsed = false,
	viewportSlide = null,
	onViewportSlideEnd,
}: ResizableChatsSidebarFrameProps) => {
	const [storedWidth] = useState(loadStoredLeftSidebarWidth);
	const [width, setWidth] = useState(() => clampLeftSidebarWidth(storedWidth));
	// The width the user chose, kept in memory because storage writes can fail.
	const userWidth = useRef(storedWidth);
	const [maxWidth, setMaxWidth] = useState(getLeftSidebarMaxWidth);
	const [isPointerResizing, setIsPointerResizing] = useState(false);
	const isDragging = useRef(false);
	const activePointerId = useRef<number | null>(null);
	const startX = useRef(0);
	const startWidth = useRef(0);

	const setUserWidth = (nextWidth: number) => {
		const clampedWidth = clampLeftSidebarWidth(nextWidth);
		userWidth.current = clampedWidth;
		setWidth(clampedWidth);
		persistLeftSidebarWidth(clampedWidth);
	};

	useEffect(() => {
		// Clamp from the user's width so a squeezed sidebar grows back.
		const handleResize = () => {
			setMaxWidth(getLeftSidebarMaxWidth());
			setWidth(clampLeftSidebarWidth(userWidth.current));
		};
		handleResize();
		globalThis.addEventListener("resize", handleResize);
		return () => globalThis.removeEventListener("resize", handleResize);
	}, []);

	const handlePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
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
		setIsPointerResizing(true);
		e.currentTarget.setPointerCapture?.(e.pointerId);
	};

	const handlePointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
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
	const handlePointerEnd = (e: React.PointerEvent<HTMLDivElement>) => {
		if (!isDragging.current || e.pointerId !== activePointerId.current) {
			return;
		}

		isDragging.current = false;
		activePointerId.current = null;
		setIsPointerResizing(false);
		if (e.currentTarget.hasPointerCapture?.(e.pointerId)) {
			e.currentTarget.releasePointerCapture?.(e.pointerId);
		}
	};

	const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
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
				"--panel-width": `${width}px`,
			}}
			onAnimationEnd={(e) => {
				if (e.target === e.currentTarget) {
					onViewportSlideEnd?.();
				}
			}}
			className={
				viewportSlide === "out"
					? "relative invisible h-full min-h-0 w-0 min-w-0 shrink-0 overflow-hidden animate-panel-slide-out"
					: cn(
							className,
							"relative sm:overflow-hidden sm:max-w-(--agents-left-sidebar-max-width)",
							!isPointerResizing &&
								"sm:transition-[width,min-width,visibility] sm:duration-(--panel-slide-duration) sm:ease-out",
							isCollapsed
								? "sm:invisible sm:w-0 sm:min-w-0"
								: "sm:w-(--agents-left-sidebar-width) sm:min-w-(--agents-left-sidebar-min-width)",
							viewportSlide === "in" && "sm:animate-panel-slide-in",
						)
			}
		>
			{/* Fixed width so content slides behind the edge instead of reflowing. */}
			<div
				className={
					viewportSlide === "out"
						? "h-full w-(--agents-left-sidebar-width)"
						: "size-full sm:w-(--agents-left-sidebar-width)"
				}
			>
				{children}
			</div>
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
