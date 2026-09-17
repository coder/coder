import { type FC, useEffect, useEffectEvent } from "react";
import type { Chat } from "#/api/typesGenerated";
import type { CardColor } from "./boardLabels";
import type { ChatWindow } from "./boardStorage";
import { FloatingChat } from "./ChatWindows";

interface BoardWindowsProps {
	readonly windows: readonly ChatWindow[];
	readonly chatsById: ReadonlyMap<string, Chat>;
	readonly colorByChatId: ReadonlyMap<string, CardColor>;
	readonly onChange: (next: ChatWindow) => void;
	readonly onClose: (chatId: string) => void;
	readonly onRaise: (chatId: string) => void;
	readonly onPreviewEnter: () => void;
	readonly onPreviewLeave: () => void;
	/** Escape outside a text field: the preview goes, else the frontmost window. */
	readonly onDismissTop: () => void;
}

export const BoardWindows: FC<BoardWindowsProps> = ({
	windows,
	chatsById,
	colorByChatId,
	onChange,
	onClose,
	onRaise,
	onPreviewEnter,
	onPreviewLeave,
	onDismissTop,
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

	return windows.map((win) => (
		<FloatingChat
			key={win.chatId}
			window={win}
			chat={chatsById.get(win.chatId)}
			color={colorByChatId.get(win.chatId)}
			onChange={onChange}
			onClose={() => onClose(win.chatId)}
			onInteract={() => onRaise(win.chatId)}
			onPreviewEnter={onPreviewEnter}
			onPreviewLeave={onPreviewLeave}
		/>
	));
};
