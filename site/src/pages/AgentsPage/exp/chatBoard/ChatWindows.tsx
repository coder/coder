import { cn } from "cn";
import { XIcon } from "lucide-react";
import {
	type FC,
	lazy,
	type PointerEvent as ReactPointerEvent,
	Suspense,
	useRef,
} from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { AgentChatPageSkeleton } from "../../components/AgentsSkeletons";
import { CARD_COLOR_CLASS, type CardColor } from "./boardLabels";
import type { ChatWindow } from "./boardStorage";

const AgentChatPage = lazy(() => import("../../AgentChatPage"));

const DEFAULT_SIZE = { width: 520, height: 640 };
const MIN_SIZE = { width: 320, height: 240 };
const MARGIN = 12;

/** A window beside `anchor`, to its right when there is room, kept on screen. */
export const windowBeside = (
	chatId: string,
	anchor: DOMRect,
	pinned: boolean,
): ChatWindow => {
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
		pinned,
	});
};

/** A pinned window in the middle of the viewport, for chats opened without a card in view. */
export const windowCentered = (chatId: string): ChatWindow => {
	const width = Math.min(DEFAULT_SIZE.width, window.innerWidth - 2 * MARGIN);
	const height = Math.min(DEFAULT_SIZE.height, window.innerHeight - 2 * MARGIN);
	return {
		chatId,
		x: (window.innerWidth - width) / 2,
		y: (window.innerHeight - height) / 2,
		width,
		height,
		pinned: true,
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
	/** New geometry after a drag or resize gesture ends. */
	readonly onChange: (next: ChatWindow) => void;
	readonly onClose: () => void;
	/** Any pointer or key interaction inside; pins a preview, raises a window. */
	readonly onInteract: () => void;
	/** Preview only: the pointer entering keeps it, leaving lets it close. */
	readonly onPreviewEnter: () => void;
	readonly onPreviewLeave: () => void;
}

/**
 * One chat floating over the board. The title bar drags it, the corner
 * handle resizes it, and geometry is committed when the gesture ends so
 * the board does not re-render per pixel. Gestures write to the DOM
 * directly meanwhile, so a preview that gets pinned mid-drag keeps going.
 */
export const FloatingChat: FC<FloatingChatProps> = ({
	window: win,
	chat,
	color,
	onChange,
	onClose,
	onInteract,
	onPreviewEnter,
	onPreviewLeave,
}) => {
	const frame = useRef<HTMLDivElement>(null);
	const colors = color ? CARD_COLOR_CLASS[color] : undefined;

	// Pointer capture keeps a gesture alive when the cursor outruns the
	// element; preventDefault stops text selection from starting under it.
	const gesture = (
		e: ReactPointerEvent<HTMLElement>,
		apply: (dx: number, dy: number) => ChatWindow,
	) => {
		if (e.button !== 0) return;
		const el = frame.current;
		if (!el) return;
		e.preventDefault();
		const handle = e.currentTarget;
		handle.setPointerCapture(e.pointerId);
		const startX = e.clientX;
		const startY = e.clientY;
		let last = win;
		const onMove = (ev: PointerEvent) => {
			last = clampWindow(apply(ev.clientX - startX, ev.clientY - startY));
			el.style.left = `${last.x}px`;
			el.style.top = `${last.y}px`;
			el.style.width = `${last.width}px`;
			el.style.height = `${last.height}px`;
		};
		const onUp = () => {
			handle.removeEventListener("pointermove", onMove);
			handle.removeEventListener("pointerup", onUp);
			if (last !== win) onChange(last);
		};
		handle.addEventListener("pointermove", onMove);
		handle.addEventListener("pointerup", onUp);
	};
	const startDrag = (e: ReactPointerEvent<HTMLElement>) =>
		gesture(e, (dx, dy) => ({ ...win, x: win.x + dx, y: win.y + dy }));
	const startResize = (e: ReactPointerEvent<HTMLElement>) =>
		gesture(e, (dx, dy) => ({
			...win,
			width: Math.max(MIN_SIZE.width, win.width + dx),
			height: Math.max(MIN_SIZE.height, win.height + dy),
		}));

	return (
		<div
			ref={frame}
			role="dialog"
			aria-label={chat?.title ?? "Chat"}
			className={cn(
				"fixed z-40 flex flex-col overflow-hidden rounded-lg border border-border bg-surface-primary shadow-[0_12px_40px_rgba(0,0,0,0.18)]",
				!win.pinned && "border-content-link/50",
			)}
			style={{
				left: win.x,
				top: win.y,
				width: win.width,
				height: win.height,
			}}
			onPointerDownCapture={onInteract}
			onKeyDownCapture={onInteract}
			onPointerEnter={win.pinned ? undefined : onPreviewEnter}
			onPointerLeave={win.pinned ? undefined : onPreviewLeave}
		>
			<div
				className="flex h-8 shrink-0 cursor-grab touch-none select-none items-center gap-2 border-b border-border bg-surface-secondary/60 pr-1 pl-3 text-[12.5px] font-medium text-content-primary active:cursor-grabbing"
				onPointerDown={startDrag}
			>
				<span
					className={cn(
						"size-2 shrink-0 rounded-[2px]",
						colors ? colors.swatch : "bg-content-secondary/30",
					)}
				/>
				<span className="min-w-0 flex-1 truncate">{chat?.title ?? "Chat"}</span>
				{!win.pinned && (
					<span className="text-[11px] font-normal text-content-secondary">
						click or drag to keep
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
			{/* Above the chat's own footer, which otherwise takes the pointer. */}
			<div
				role="presentation"
				aria-hidden="true"
				className="absolute right-0 bottom-0 z-10 size-4 cursor-nwse-resize touch-none [background:linear-gradient(135deg,transparent_50%,var(--color-border)_50%,var(--color-border)_60%,transparent_60%,transparent_75%,var(--color-border)_75%,var(--color-border)_85%,transparent_85%)]"
				onPointerDown={startResize}
			/>
		</div>
	);
};
