import { lazy, Suspense, useEffect, useEffectEvent } from "react";
import type { Chat } from "#/api/typesGenerated";
import { AgentChatPageSkeleton } from "../../components/AgentsSkeletons";
import type { LaneProject } from "./BoardLane";
import {
	type BoardState,
	cardContext,
	cardOf,
	cardOfChat,
	newChatLabels,
} from "./boardApi";
import { type BoardCard, type CardColor, laneOf } from "./boardLabels";
import { type ChatWindow, type DraftTarget, windowKey } from "./boardStorage";
import { FloatingChat } from "./ChatWindows";
import { DraftChat } from "./DraftChat";
import { type WindowTab, WindowTabs } from "./WindowTabs";

// Lazy so the board loads without the chat page.
const AgentChatPage = lazy(() => import("../../AgentChatPage/AgentChatPage"));

type BoardWindowsProps = {
	readonly windows: readonly ChatWindow[];
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly colorByChatId: ReadonlyMap<string, CardColor>;
	/** The unfiltered model a draft is born into. */
	readonly board: BoardState;
	readonly onChange: (next: ChatWindow) => void;
	readonly onClose: (key: string) => void;
	readonly onMinimize: (key: string) => void;
	/** A tab was clicked: the minimized window comes back in front. */
	readonly onRestore: (key: string) => void;
	readonly onRaise: (key: string) => void;
	readonly onPreviewEnter: () => void;
	readonly onPreviewLeave: () => void;
	/** Escape outside a text field: the preview goes, else the frontmost window. */
	readonly onDismissTop: () => void;
	readonly onDraftCreated: (target: DraftTarget, chatId: string) => void;
	readonly onCardAssistant: (card: BoardCard) => void;
	/** The project a lane files new chats into; undefined for none or unknown. */
	readonly projectOf: (lane: string) => LaneProject | null | undefined;
};

/** What a window shows in its title bar and in its tab once minimized. */
const summarizeWindow = (
	win: ChatWindow,
	board: BoardState,
	chatsById: ReadonlyMap<string, Chat>,
	colorByChatId: ReadonlyMap<string, CardColor>,
	projectOf: (lane: string) => LaneProject | null | undefined,
): Omit<WindowTab, "key"> => {
	if (win.kind === "chat") {
		const chat = chatsById.get(win.chatId);
		return {
			title: chat?.title ?? "Chat",
			color: colorByChatId.get(win.chatId),
			chat,
		};
	}

	const { target } = win;
	if ("column" in target) {
		// Names the project too, so the window says where the chat will land.
		const project = target.lane ? projectOf(target.lane) : undefined;
		const where =
			project === undefined ? "" : `${project?.name ?? "No project"} / `;
		return {
			title: `New chat in ${where}${target.column}`,
			color: undefined,
			chat: undefined,
		};
	}

	const card = cardOf(board, target.cardId);
	return {
		title: `New chat in ${card?.title ?? "card"}`,
		color: card?.color,
		chat: undefined,
	};
};

export const BoardWindows: React.FC<BoardWindowsProps> = ({
	windows,
	chatsById,
	colorByChatId,
	board,
	onChange,
	onClose,
	onMinimize,
	onRestore,
	onRaise,
	onPreviewEnter,
	onPreviewLeave,
	onDismissTop,
	onDraftCreated,
	onCardAssistant,
	projectOf,
}) => {
	const hasWindows = windows.length > 0;
	const dismissTop = useEffectEvent(onDismissTop);
	useEffect(() => {
		if (!hasWindows) return;
		const onKey = (e: KeyboardEvent) => {
			const target = e.target instanceof HTMLElement ? e.target : null;
			const typing =
				target?.tagName === "INPUT" ||
				target?.tagName === "TEXTAREA" ||
				target?.isContentEditable;
			if (e.key === "Escape" && !typing) dismissTop();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [hasWindows]);

	const frames = windows.map((win) => {
		const key = windowKey(win);
		const { title, color } = summarizeWindow(
			win,
			board,
			chatsById,
			colorByChatId,
			projectOf,
		);
		const frame = {
			window: win,
			title,
			color,
			onChange,
			onClose: () => onClose(key),
			onMinimize: () => onMinimize(key),
			onInteract: () => onRaise(key),
			onPreviewEnter,
			onPreviewLeave,
		};

		if (win.kind === "chat") {
			// Assistant chats are on no card, so they get no assistant button.
			const card = cardOfChat(board, win.chatId);
			return (
				<FloatingChat
					key={key}
					{...frame}
					cardAssistant={
						card && {
							cardTitle: card.title,
							open: () => onCardAssistant(card),
						}
					}
				>
					<Suspense fallback={<AgentChatPageSkeleton />}>
						<AgentChatPage chatId={win.chatId} />
					</Suspense>
				</FloatingChat>
			);
		}

		const { target } = win;
		const labels = newChatLabels(board, target);
		// The page closes a draft whose card is gone; until that commits there
		// is nothing to draw.
		if (!labels) return null;

		const card =
			"cardId" in target
				? board.cards.find((c) => c.id === target.cardId)
				: undefined;
		const lane = card
			? laneOf(card)
			: "lane" in target
				? target.lane
				: undefined;
		const project = lane === undefined ? undefined : projectOf(lane);
		return (
			<FloatingChat
				// Keyed by target so a new target gets a fresh form, not the pending
				// request, text or error of the draft it replaced.
				key={
					"cardId" in target
						? `${key}:card:${target.cardId}`
						: `${key}:column:${target.lane ?? ""}:${target.column}`
				}
				{...frame}
			>
				<DraftChat
					labels={labels}
					context={
						card && win.includeCardContext ? cardContext(card) : undefined
					}
					project={project}
					onCreated={(chatId) => onDraftCreated(target, chatId)}
				/>
			</FloatingChat>
		);
	});

	// Newest first: the first minimized tab sits at the right edge and each
	// later one lands to its left, nearest the board.
	const tabs = windows
		.filter((win) => win.minimized)
		.map((win) => ({
			key: windowKey(win),
			...summarizeWindow(win, board, chatsById, colorByChatId, projectOf),
		}))
		.toReversed();

	return (
		<>
			{frames}
			<WindowTabs tabs={tabs} onRestore={onRestore} onClose={onClose} />
		</>
	);
};
