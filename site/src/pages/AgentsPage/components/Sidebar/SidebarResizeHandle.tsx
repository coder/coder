import { type FC, type PointerEvent as ReactPointerEvent, useRef } from "react";

interface SidebarResizeHandleProps {
	width: number;
	setWidth: (w: number) => void;
	minWidth: number;
	maxWidth: number;
}

export const SidebarResizeHandle: FC<SidebarResizeHandleProps> = ({
	width,
	setWidth,
	minWidth,
	maxWidth,
}) => {
	// Refs instead of state so drag updates never trigger re-renders of this
	// component itself, only the parent's setWidth call does.
	const isDragging = useRef(false);
	const startX = useRef(0);
	const startWidth = useRef(0);

	const handlePointerDown = (e: ReactPointerEvent<HTMLDivElement>) => {
		e.preventDefault();
		isDragging.current = true;
		startX.current = e.clientX;
		startWidth.current = width;
		(e.target as HTMLElement).setPointerCapture(e.pointerId);
		// Prevent text selection across the whole page while dragging.
		document.body.classList.add("select-none");
	};

	const handlePointerMove = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}
		const delta = e.clientX - startX.current;
		const raw = startWidth.current + delta;
		setWidth(Math.min(maxWidth, Math.max(minWidth, raw)));
	};

	const handlePointerUp = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (!isDragging.current) {
			return;
		}
		isDragging.current = false;
		(e.target as HTMLElement).releasePointerCapture(e.pointerId);
		document.body.classList.remove("select-none");
	};

	return (
		// The outer div is the hit zone: 8px padding on each side of the 1px
		// line (total 17px), translated right by half so it straddles the edge.
		<div
			onPointerDown={handlePointerDown}
			onPointerMove={handlePointerMove}
			onPointerUp={handlePointerUp}
			className="group/resize absolute right-0 top-0 z-20 h-full w-[17px] translate-x-1/2 cursor-col-resize"
		>
			{/* Visible line, centered inside the hit zone. Expands and changes
			    color on hover to give users a clear affordance. */}
			<div className="mx-auto h-full w-px bg-border transition-all group-hover/resize:w-0.5 group-hover/resize:bg-content-link" />
		</div>
	);
};
