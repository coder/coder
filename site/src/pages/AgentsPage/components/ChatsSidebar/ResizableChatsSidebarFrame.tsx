import {
	type KeyboardEvent as ReactKeyboardEvent,
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	useEffect,
	useEffectEvent,
	useRef,
	useState,
} from "react";
import { cn } from "#/utils/cn";
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
		e.preventDefault();
		isDragging.current = true;
		startX.current = e.clientX;
		startWidth.current = width;
		e.currentTarget.setPointerCapture?.(e.pointerId);
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}

		const rawWidth = startWidth.current + (e.clientX - startX.current);
		setUserWidth(rawWidth);
	};

	const handlePointerEnd = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}

		isDragging.current = false;
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
				"relative sm:w-[var(--agents-left-sidebar-width)] sm:min-w-[var(--agents-left-sidebar-min-width)] sm:max-w-[var(--agents-left-sidebar-max-width)]",
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
				onKeyDown={handleKeyDown}
				className="absolute top-0 right-0 z-20 hidden h-full w-1 touch-none cursor-col-resize select-none transition-colors hover:bg-content-link focus-visible:bg-content-link focus-visible:outline-none sm:block"
			/>
		</div>
	);
};
