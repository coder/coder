import { cn } from "cn";
import {
	type FC,
	type KeyboardEvent,
	type PointerEvent,
	type RefObject,
	useRef,
	useState,
} from "react";
import { COLLAPSED_WIDTH, EXPANDED_WIDTH } from "./useSidebarResize";

/** Max pointer travel in px for a press to count as a click. */
const CLICK_DEAD_ZONE = 3;

type DragState = {
	startX: number;
	startLeft: number;
	/** Past the click dead zone. */
	moved: boolean;
	/** Last previewed width. */
	width: number;
};

type SidebarResizeHandleProps = {
	/** Element whose width is previewed during a drag. */
	containerRef: RefObject<HTMLElement | null>;
	collapsed: boolean;
	onCollapse: () => void;
	onExpand: () => void;
};

/** Sidebar edge: click toggles, drag snaps on release, arrow keys resize. */
export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	containerRef,
	collapsed,
	onCollapse,
	onExpand,
}) => {
	const [resizing, setResizing] = useState(false);
	const dragRef = useRef<DragState | null>(null);

	const handlePointerDown = (e: PointerEvent<HTMLDivElement>) => {
		const container = containerRef.current;
		if (e.button !== 0 || !container) {
			return;
		}
		// Prevent text selection.
		e.preventDefault();
		dragRef.current = {
			startX: e.clientX,
			startLeft: container.getBoundingClientRect().left,
			moved: false,
			width: collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH,
		};
		setResizing(true);
		e.currentTarget.setPointerCapture?.(e.pointerId);
	};

	const handlePointerMove = (e: PointerEvent<HTMLDivElement>) => {
		const drag = dragRef.current;
		const container = containerRef.current;
		if (!drag || !container) {
			return;
		}
		if (!drag.moved) {
			if (Math.abs(e.clientX - drag.startX) < CLICK_DEAD_ZONE) {
				return;
			}
			drag.moved = true;
			// Width follows the pointer without animating.
			container.style.transition = "none";
		}
		drag.width = Math.max(
			COLLAPSED_WIDTH,
			Math.min(e.clientX - drag.startLeft, EXPANDED_WIDTH),
		);
		container.style.width = `${drag.width}px`;
	};

	// Only pointerup commits; cancel and lost capture revert.
	const finishDrag = (e: PointerEvent<HTMLDivElement>, commit: boolean) => {
		const drag = dragRef.current;
		if (!drag) {
			return;
		}
		dragRef.current = null;
		setResizing(false);
		if (e.currentTarget.hasPointerCapture?.(e.pointerId)) {
			e.currentTarget.releasePointerCapture?.(e.pointerId);
		}

		if (!drag.moved) {
			if (!commit) {
				return;
			}
			if (collapsed) {
				onExpand();
			} else {
				onCollapse();
			}
			return;
		}

		const container = containerRef.current;
		if (!container) {
			return;
		}
		// Snap in the drag direction.
		let shouldCollapse = collapsed;
		if (commit) {
			shouldCollapse = collapsed
				? drag.width <= COLLAPSED_WIDTH
				: drag.width < EXPANDED_WIDTH;
		}
		container.style.transition = "";
		container.style.width = `${shouldCollapse ? COLLAPSED_WIDTH : EXPANDED_WIDTH}px`;
		if (shouldCollapse !== collapsed) {
			if (shouldCollapse) {
				onCollapse();
			} else {
				onExpand();
			}
		}
	};

	const handleKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
		switch (e.key) {
			case "ArrowLeft":
			case "Home":
				e.preventDefault();
				onCollapse();
				break;
			case "ArrowRight":
			case "End":
				e.preventDefault();
				onExpand();
				break;
		}
	};

	return (
		<div
			role="separator"
			aria-orientation="vertical"
			aria-label="Resize sidebar"
			aria-valuemin={COLLAPSED_WIDTH}
			aria-valuemax={EXPANDED_WIDTH}
			aria-valuenow={collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH}
			tabIndex={0}
			onPointerDown={handlePointerDown}
			onPointerMove={handlePointerMove}
			onPointerUp={(e) => finishDrag(e, true)}
			onPointerCancel={(e) => finishDrag(e, false)}
			onLostPointerCapture={(e) => finishDrag(e, false)}
			onKeyDown={handleKeyDown}
			className="group absolute top-0 -right-2 z-10 h-full w-4 cursor-col-resize touch-none select-none outline-hidden"
		>
			<div
				className={cn(
					"absolute top-0 left-[7px] h-full w-0.5 rounded-full bg-border",
					"opacity-0 transition-opacity duration-150",
					"group-hover:opacity-100 group-focus-visible:opacity-100",
					resizing && "opacity-100",
				)}
			/>
		</div>
	);
};
