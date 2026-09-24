import { cn } from "cn";
import { XIcon } from "lucide-react";
import {
	type FC,
	lazy,
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
		window.addEventListener("pointermove", onMove);
		window.addEventListener("pointerup", onUp);
		return () => {
			if (frame !== null) cancelAnimationFrame(frame);
			window.removeEventListener("pointermove", onMove);
			window.removeEventListener("pointerup", onUp);
		};
	}, [gesture]);

	// preventDefault stops text selection from starting under the handle.
	const start = (kind: Gesture["kind"]) => (e: ReactPointerEvent) => {
		if (e.button !== 0) return;
		e.preventDefault();
		setGesture({ kind, startX: e.clientX, startY: e.clientY, origin: win });
	};

	return (
		<div
			role="dialog"
			aria-label={chat?.title ?? "Chat"}
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
				<span
					className={cn(
						"size-2 shrink-0 rounded-[2px]",
						color ? cardSwatch({ color }) : "bg-content-secondary/30",
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
				onPointerDown={start("resize")}
			/>
		</div>
	);
};
