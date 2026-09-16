import { cn } from "cn";
import { XIcon } from "lucide-react";
import {
	type FC,
	lazy,
	type PointerEvent as ReactPointerEvent,
	Suspense,
	useEffect,
	useRef,
} from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { AgentChatPageSkeleton } from "../AgentsSkeletons";
import { CARD_COLOR_CLASS, type CardColor } from "./boardLabels";
import type { ChatWindow } from "./boardStorage";

const AgentChatPage = lazy(() => import("../../AgentChatPage"));

const DEFAULT_SIZE = { width: 520, height: 640 };
const MARGIN = 12;

/** A window beside `anchor`, to its right when there is room, kept on screen. */
export const windowBeside = (chatId: string, anchor: DOMRect): ChatWindow => {
	const { width, height } = DEFAULT_SIZE;
	const vw = window.innerWidth;
	const vh = window.innerHeight;
	const fitsRight = anchor.right + 8 + width <= vw - MARGIN;
	const x = fitsRight ? anchor.right + 8 : anchor.left - 8 - width;
	return clampWindow({
		chatId,
		x,
		y: anchor.top,
		width: Math.min(width, vw - 2 * MARGIN),
		height: Math.min(height, vh - 2 * MARGIN),
	});
};

/** A window in the middle of the viewport, for chats opened without a card in view. */
export const windowCentered = (chatId: string): ChatWindow => {
	const width = Math.min(DEFAULT_SIZE.width, window.innerWidth - 2 * MARGIN);
	const height = Math.min(DEFAULT_SIZE.height, window.innerHeight - 2 * MARGIN);
	return {
		chatId,
		x: (window.innerWidth - width) / 2,
		y: (window.innerHeight - height) / 2,
		width,
		height,
	};
};

const clampWindow = (w: ChatWindow): ChatWindow => ({
	...w,
	x: Math.max(MARGIN, Math.min(w.x, window.innerWidth - w.width - MARGIN)),
	y: Math.max(MARGIN, Math.min(w.y, window.innerHeight - w.height - MARGIN)),
});

interface FloatingChatProps {
	readonly window: ChatWindow;
	readonly chat: Chat | undefined;
	readonly color: CardColor | undefined;
	/** A hover preview: not yet pinned, closes when the pointer leaves. */
	readonly preview: boolean;
	readonly onChange: (next: ChatWindow) => void;
	readonly onClose: () => void;
	/** Any pointer or key interaction inside; pins a preview, raises a window. */
	readonly onInteract: () => void;
	readonly onPointerEnter: () => void;
	readonly onPointerLeave: () => void;
}

/**
 * One chat floating over the board. The title bar drags it, the corner
 * handle resizes it (native CSS resize), and geometry is committed to
 * storage when the gesture ends so the board does not re-render per pixel.
 */
export const FloatingChat: FC<FloatingChatProps> = ({
	window: win,
	chat,
	color,
	preview,
	onChange,
	onClose,
	onInteract,
	onPointerEnter,
	onPointerLeave,
}) => {
	const frame = useRef<HTMLDivElement>(null);
	const colors = color ? CARD_COLOR_CLASS[color] : undefined;

	// The native resize handle gives no end event, so observe size changes
	// and commit them once the pointer is released.
	useEffect(() => {
		const el = frame.current;
		if (!el) return;
		let size: { width: number; height: number } | null = null;
		const observer = new ResizeObserver(([entry]) => {
			if (!entry) return;
			const { width, height } = entry.target.getBoundingClientRect();
			if (width !== win.width || height !== win.height)
				size = { width, height };
		});
		observer.observe(el);
		const commit = () => {
			if (size) onChange(clampWindow({ ...win, ...size }));
			size = null;
		};
		document.addEventListener("pointerup", commit);
		return () => {
			observer.disconnect();
			document.removeEventListener("pointerup", commit);
		};
	}, [win, onChange]);

	// Pointer capture keeps the drag alive when the cursor outruns the bar.
	const startDrag = (e: ReactPointerEvent<HTMLDivElement>) => {
		if (e.button !== 0) return;
		const el = frame.current;
		if (!el) return;
		const bar = e.currentTarget;
		bar.setPointerCapture(e.pointerId);
		const startX = e.clientX - win.x;
		const startY = e.clientY - win.y;
		let last = win;
		const onMove = (ev: PointerEvent) => {
			last = clampWindow({
				...win,
				x: ev.clientX - startX,
				y: ev.clientY - startY,
			});
			el.style.left = `${last.x}px`;
			el.style.top = `${last.y}px`;
		};
		const onUp = () => {
			bar.removeEventListener("pointermove", onMove);
			bar.removeEventListener("pointerup", onUp);
			if (last !== win) onChange(last);
		};
		bar.addEventListener("pointermove", onMove);
		bar.addEventListener("pointerup", onUp);
	};

	return (
		<div
			ref={frame}
			role="dialog"
			aria-label={chat?.title ?? "Chat"}
			className={cn(
				"fixed z-40 flex min-h-60 min-w-80 flex-col overflow-hidden rounded-lg border border-border bg-surface-primary shadow-[0_12px_40px_rgba(0,0,0,0.18)] [resize:both]",
				preview && "border-content-link/50",
			)}
			style={{
				left: win.x,
				top: win.y,
				width: win.width,
				height: win.height,
			}}
			onPointerDownCapture={onInteract}
			onKeyDownCapture={onInteract}
			onPointerEnter={onPointerEnter}
			onPointerLeave={onPointerLeave}
		>
			<div
				className="flex h-8 shrink-0 cursor-grab touch-none items-center gap-2 border-b border-border bg-surface-secondary/60 pr-1 pl-3 text-[12.5px] font-medium text-content-primary active:cursor-grabbing"
				onPointerDown={startDrag}
			>
				<span
					className={cn(
						"size-2 shrink-0 rounded-[2px]",
						colors ? colors.swatch : "bg-content-secondary/30",
					)}
				/>
				<span className="min-w-0 flex-1 truncate">{chat?.title ?? "Chat"}</span>
				{preview && (
					<span className="text-[11px] font-normal text-content-secondary">
						click to keep
					</span>
				)}
				<Button
					variant="subtle"
					size="icon"
					aria-label={`Close ${chat?.title ?? "chat"}`}
					className="size-6 shrink-0 text-content-secondary hover:text-content-primary"
					onPointerDown={(e) => e.stopPropagation()}
					onClick={onClose}
				>
					<XIcon className="size-3.5" />
				</Button>
			</div>
			<div className="flex min-h-0 flex-1 flex-col">
				<Suspense fallback={<AgentChatPageSkeleton />}>
					<AgentChatPage chatId={win.chatId} />
				</Suspense>
			</div>
		</div>
	);
};
