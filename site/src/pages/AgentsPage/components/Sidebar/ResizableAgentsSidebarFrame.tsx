import {
	type CSSProperties,
	type KeyboardEvent as ReactKeyboardEvent,
	type ReactNode,
	type PointerEvent as ReactPointerEvent,
	useEffect,
	useRef,
	useState,
} from "react";
import { cn } from "#/utils/cn";
import {
	clampLeftSidebarWidth,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_MIN_WIDTH,
	loadPersistedLeftSidebarWidth,
	persistLeftSidebarWidth,
} from "./sidebarWidth";

const KEYBOARD_RESIZE_STEP = 16;

interface ResizableAgentsSidebarFrameProps {
	children: ReactNode;
	className?: string;
}

export const ResizableAgentsSidebarFrame = ({
	children,
	className,
}: ResizableAgentsSidebarFrameProps) => {
	const [width, setWidth] = useState(loadInitialWidth);
	const [maxWidth, setMaxWidth] = useState(getLeftSidebarMaxWidth);
	const isDragging = useRef(false);
	const startX = useRef(0);
	const startWidth = useRef(0);

	useEffect(() => {
		const handleResize = () => {
			setMaxWidth(getLeftSidebarMaxWidth());
			setWidth((currentWidth) => clampLeftSidebarWidth(currentWidth));
		};

		addEventListener("resize", handleResize);
		return () => removeEventListener("resize", handleResize);
	}, []);

	useEffect(() => {
		persistLeftSidebarWidth(width);
	}, [width]);

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
		setWidth(clampLeftSidebarWidth(rawWidth));
	};

	const handlePointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
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
				setWidth((currentWidth) =>
					clampLeftSidebarWidth(currentWidth - KEYBOARD_RESIZE_STEP),
				);
				break;
			case "ArrowRight":
				e.preventDefault();
				setWidth((currentWidth) =>
					clampLeftSidebarWidth(currentWidth + KEYBOARD_RESIZE_STEP),
				);
				break;
			case "Home":
				e.preventDefault();
				setWidth(LEFT_SIDEBAR_MIN_WIDTH);
				break;
			case "End":
				e.preventDefault();
				setWidth(maxWidth);
				break;
		}
	};

	return (
		<div
			data-testid="agents-left-sidebar"
			style={
				{
					"--agents-left-sidebar-width": `${width}px`,
				} as CSSProperties
			}
			className={cn(
				className,
				"relative md:w-[var(--agents-left-sidebar-width)] md:min-w-[240px] md:max-w-[min(520px,50vw)]",
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
				data-testid="agents-left-sidebar-resize-handle"
				onPointerDown={handlePointerDown}
				onPointerMove={handlePointerMove}
				onPointerUp={handlePointerUp}
				onPointerCancel={handlePointerUp}
				onKeyDown={handleKeyDown}
				className="absolute top-0 right-0 z-20 hidden h-full w-1 cursor-col-resize select-none transition-colors hover:bg-content-link focus-visible:bg-content-link focus-visible:outline-none md:block"
			/>
		</div>
	);
};

function loadInitialWidth(): number {
	return clampLeftSidebarWidth(loadPersistedLeftSidebarWidth());
}
