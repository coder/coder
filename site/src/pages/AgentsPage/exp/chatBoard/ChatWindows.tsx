import { cn } from "cn";
import { XIcon } from "lucide-react";
import {
	type FC,
	lazy,
	type KeyboardEvent as ReactKeyboardEvent,
	type PointerEvent as ReactPointerEvent,
	Suspense,
	useEffect,
	useEffectEvent,
	useState,
} from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { AgentChatPageSkeleton } from "../../components/AgentsSkeletons";
import type { CardColor } from "./boardLabels";
import type { ChatWindow } from "./boardStorage";
import { cardSwatch } from "./cardColor";
import { clampWindow, MIN_WINDOW_SIZE } from "./windows";

const AgentChatPage = lazy(() => import("../../AgentChatPage"));

/** How far one arrow press moves or resizes a window. */
const KEY_STEP_PX = 16;

const ARROW_DELTAS: Readonly<Record<string, readonly [number, number]>> = {
	ArrowUp: [0, -KEY_STEP_PX],
	ArrowDown: [0, KEY_STEP_PX],
	ArrowLeft: [-KEY_STEP_PX, 0],
	ArrowRight: [KEY_STEP_PX, 0],
};

/** A drag of the title bar or the resize corner, from where the pointer went down. */
type Gesture = {
	readonly kind: "move" | "resize";
	readonly startX: number;
	readonly startY: number;
	readonly origin: ChatWindow;
};

const applyGesture = (
	gesture: Gesture,
	clientX: number,
	clientY: number,
): ChatWindow => {
	const dx = clientX - gesture.startX;
	const dy = clientY - gesture.startY;
	const { origin } = gesture;
	if (gesture.kind === "move") {
		return clampWindow({ ...origin, x: origin.x + dx, y: origin.y + dy });
	}
	return clampWindow({
		...origin,
		width: Math.max(MIN_WINDOW_SIZE.width, origin.width + dx),
		height: Math.max(MIN_WINDOW_SIZE.height, origin.height + dy),
	});
};

type FloatingChatProps = {
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
};

/**
 * One chat floating over the board. The title bar drags it, the corner
 * handle resizes it. Geometry is committed when the gesture ends so the
 * board does not re-render per pixel; meanwhile only this window follows
 * the pointer.
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
	const [gesture, setGesture] = useState<Gesture | null>(null);
	const [live, setLive] = useState<ChatWindow | null>(null);
	const shown = live ?? win;

	// Listeners on the window, not the handle: the pointer outruns the
	// element mid-gesture. Moves are drawn once per frame; the release
	// commits the latest position whether or not that frame ran. Committed
	// through an event so the latest onChange is used even though the
	// effect only re-runs when the gesture changes.
	const commit = useEffectEvent((next: ChatWindow) => onChange(next));
	useEffect(() => {
		if (!gesture) return;
		let last = gesture.origin;
		let frame: number | null = null;
		const onMove = (e: PointerEvent) => {
			last = applyGesture(gesture, e.clientX, e.clientY);
			if (frame !== null) return;
			frame = requestAnimationFrame(() => {
				frame = null;
				setLive(last);
			});
		};
		const onUp = () => {
			setGesture(null);
			setLive(null);
			if (last !== gesture.origin) commit(last);
		};
		// A cancelled pointer (touch scroll, browser gesture) ends the drag
		// where the window was; nothing is committed.
		const onCancel = () => {
			setGesture(null);
			setLive(null);
		};
		window.addEventListener("pointermove", onMove);
		window.addEventListener("pointerup", onUp);
		window.addEventListener("pointercancel", onCancel);
		return () => {
			if (frame !== null) cancelAnimationFrame(frame);
			window.removeEventListener("pointermove", onMove);
			window.removeEventListener("pointerup", onUp);
			window.removeEventListener("pointercancel", onCancel);
		};
	}, [gesture]);

	// preventDefault stops text selection from starting under the handle.
	const start = (kind: Gesture["kind"]) => (e: ReactPointerEvent) => {
		if (e.button !== 0) return;
		e.preventDefault();
		setGesture({ kind, startX: e.clientX, startY: e.clientY, origin: win });
	};

	// Arrows move, Shift+arrows resize, one step per press; the same clamp
	// and minimum size as a pointer gesture from a zero origin.
	const onKeyDown = (e: ReactKeyboardEvent) => {
		const delta = ARROW_DELTAS[e.key];
		if (!delta) return;
		e.preventDefault();
		const kind = e.shiftKey ? "resize" : "move";
		onChange(
			applyGesture({ kind, startX: 0, startY: 0, origin: win }, ...delta),
		);
	};

	const title = chat?.title ?? "Chat";

	return (
		<div
			role="dialog"
			aria-label={title}
			className={cn(
				"fixed z-40 flex flex-col overflow-hidden rounded-lg border border-border bg-surface-primary shadow-[0_12px_40px_rgba(0,0,0,0.18)]",
				!win.pinned && "border-content-link/50",
			)}
			style={{
				left: shown.x,
				top: shown.y,
				width: shown.width,
				height: shown.height,
			}}
			onPointerDownCapture={onInteract}
			onKeyDownCapture={onInteract}
			onPointerEnter={win.pinned ? undefined : onPreviewEnter}
			onPointerLeave={win.pinned ? undefined : onPreviewLeave}
		>
			<div
				className="flex h-8 shrink-0 cursor-grab touch-none select-none items-center gap-2 border-b border-border bg-surface-secondary/60 pr-1 pl-3 text-[12.5px] font-medium text-content-primary active:cursor-grabbing"
				onPointerDown={start("move")}
			>
				{/* The bar's pointerdown handles drags (its preventDefault keeps a click from focusing this); the button gives the keyboard a target. */}
				<button
					type="button"
					aria-label={`Move or resize ${title}`}
					aria-keyshortcuts="ArrowUp ArrowDown ArrowLeft ArrowRight Shift+ArrowUp Shift+ArrowDown Shift+ArrowLeft Shift+ArrowRight"
					className="flex min-w-0 flex-1 cursor-grab items-center gap-2 border-0 bg-transparent p-0 text-left active:cursor-grabbing"
					onKeyDown={onKeyDown}
				>
					<span
						className={cn(
							"size-2 shrink-0 rounded-[2px]",
							color ? cardSwatch({ color }) : "bg-content-secondary/30",
						)}
					/>
					<span className="min-w-0 flex-1 truncate">{title}</span>
					{!win.pinned && (
						<span className="text-[11px] font-normal text-content-secondary">
							click or drag to keep
						</span>
					)}
				</button>
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
				onPointerDown={start("resize")}
			/>
		</div>
	);
};
