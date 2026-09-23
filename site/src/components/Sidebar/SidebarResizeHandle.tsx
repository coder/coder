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

/** Pointer travel in px below which a press and release counts as a click. */
const CLICK_DEAD_ZONE = 3;

interface DragState {
	startX: number;
	startLeft: number;
	/** Whether the pointer has left the click dead zone. */
	moved: boolean;
	/** Width last previewed on the container. */
	width: number;
}

interface SidebarResizeHandleProps {
	/** The sidebar column whose width the drag previews live. */
	containerRef: RefObject<HTMLElement | null>;
	collapsed: boolean;
	onCollapse: () => void;
	onExpand: () => void;
}

/**
 * Hit area on the sidebar's right edge that reveals a 2px line on hover,
 * focus, and while dragging. A drag previews the width directly on the
 * container and snaps to the collapsed or expanded state on release; a
 * click toggles. Keyboard users move between the two states with the
 * arrow keys, matching the other vertical separators in the app.
 */
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
		// Stops the browser from starting a text selection with the drag.
		e.preventDefault();
		dragRef.current = {
			startX: e.clientX,
			startLeft: container.getBoundingClientRect().left,
			moved: false,
			width: collapsed ? COLLAPSED_WIDTH : EXPANDED_WIDTH,
		};
		setResizing(true);
		// Capture keeps move and release events flowing to this element
		// even when the pointer leaves it or the window.
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
			// Only suppress the width transition once this is a real drag,
			// so a click still animates through React state.
			container.style.transition = "none";
		}
		drag.width = Math.max(
			COLLAPSED_WIDTH,
			Math.min(e.clientX - drag.startLeft, EXPANDED_WIDTH),
		);
		container.style.width = `${drag.width}px`;
	};

	// pointerup commits the gesture. pointercancel, and the implicit capture
	// release that follows every end, only tear it down; the release after a
	// pointerup finds no drag and is a no-op.
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
		// Snap in the direction of the drag, or back to the current state when
		// the gesture was canceled or clamped at the edge it started from. The
		// transition comes back so the snap animates.
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
