import { lazy, Suspense, useEffect, useEffectEvent } from "react";
import type { Chat } from "#/api/typesGenerated";
import { AgentChatPageSkeleton } from "../../components/AgentsSkeletons";
import {
	type BoardState,
	cardContext,
	cardOfChat,
	newChatLabels,
} from "./boardApi";
import type { BoardCard, CardColor } from "./boardLabels";
import { type ChatWindow, type DraftTarget, windowKey } from "./boardStorage";
import { FloatingChat } from "./ChatWindows";
import { DraftChat } from "./DraftChat";
import { WindowTabs } from "./WindowTabs";

// Lazy so the board loads without the chat page.
const AgentChatPage = lazy(() => import("../../AgentChatPage"));

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

	// Title and color are shared by a window's title bar and its tab.
	const heading = (win: ChatWindow) => {
		if (win.kind === "chat") {
			return {
				title: chatsById.get(win.chatId)?.title ?? "Chat",
				color: colorByChatId.get(win.chatId),
			};
		}
		const { target } = win;
		const card =
			"cardId" in target
				? board.cards.find((c) => c.id === target.cardId)
				: undefined;
		return {
			title: `New chat in ${"column" in target ? target.column : (card?.title ?? "card")}`,
			color: card?.color,
		};
	};

	const frames = windows.map((win) => {
		const key = windowKey(win);
		const frame = {
			window: win,
			...heading(win),
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
		return (
			<FloatingChat
				// Keyed by target so a new target gets a fresh form, not the pending
				// request, text or error of the draft it replaced.
				key={
					"cardId" in target
						? `${key}:card:${target.cardId}`
						: `${key}:column:${target.column}`
				}
				{...frame}
			>
				<DraftChat
					labels={labels}
					context={
						card && win.includeCardContext ? cardContext(card) : undefined
					}
					onCreated={(chatId) => onDraftCreated(target, chatId)}
				/>
			</FloatingChat>
		);
	});

	return (
		<>
			{frames}
			<WindowTabs
				tabs={windows.flatMap((win) =>
					win.minimized
						? [
								{
									key: windowKey(win),
									...heading(win),
									chat:
										win.kind === "chat" ? chatsById.get(win.chatId) : undefined,
								},
							]
						: [],
				)}
				onRestore={onRestore}
				onClose={onClose}
			/>
		</>
	);
};
