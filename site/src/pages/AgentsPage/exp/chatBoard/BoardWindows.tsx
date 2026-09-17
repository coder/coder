import { type FC, useEffect, useEffectEvent } from "react";
import type { Chat } from "#/api/typesGenerated";
import { type BoardState, cardContext, newChatLabels } from "./boardApi";
import type { CardColor } from "./boardLabels";
import { type ChatWindow, type DraftTarget, windowKey } from "./boardStorage";
import { ChatBody, FloatingChat } from "./ChatWindows";
import { DraftChat } from "./DraftChat";

interface BoardWindowsProps {
	readonly windows: readonly ChatWindow[];
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly colorByChatId: ReadonlyMap<string, CardColor>;
	/** The unfiltered model a draft is born into. */
	readonly board: BoardState;
	readonly onChange: (next: ChatWindow) => void;
	readonly onClose: (key: string) => void;
	readonly onRaise: (key: string) => void;
	readonly onPreviewEnter: () => void;
	readonly onPreviewLeave: () => void;
	/** Escape outside a text field: the preview goes, else the frontmost window. */
	readonly onDismissTop: () => void;
	readonly onDraftCreated: (target: DraftTarget, chatId: string) => void;
}

export const BoardWindows: FC<BoardWindowsProps> = ({
	windows,
	chatsById,
	colorByChatId,
	board,
	onChange,
	onClose,
	onRaise,
	onPreviewEnter,
	onPreviewLeave,
	onDismissTop,
	onDraftCreated,
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

	return windows.map((win) => {
		const key = windowKey(win);
		const frame = {
			window: win,
			onChange,
			onClose: () => onClose(key),
			onInteract: () => onRaise(key),
			onPreviewEnter,
			onPreviewLeave,
		};
		if (win.kind === "chat") {
			return (
				<FloatingChat
					key={key}
					{...frame}
					title={chatsById.get(win.chatId)?.title ?? "Chat"}
					color={colorByChatId.get(win.chatId)}
				>
					<ChatBody chatId={win.chatId} />
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
				key={key}
				{...frame}
				title={`New chat in ${"column" in target ? target.column : (card?.title ?? "card")}`}
				color={card?.color}
			>
				<DraftChat
					labels={labels}
					context={card && win.withContext ? cardContext(card) : undefined}
					onCreated={(chatId) => onDraftCreated(target, chatId)}
				/>
			</FloatingChat>
		);
	});
};
