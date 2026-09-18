import { cn } from "cn";
import { BotIcon, MessageSquareIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import { IconButton } from "./IconButton";

/** Inside a card or row the anchor is fixed, so its openers pass only the chat. */
interface ChatOpeners {
	readonly onOpen: (chat: Chat) => void;
	readonly onPreview: (chat: Chat) => void;
	readonly onPreviewEnd: () => void;
}

/**
 * Unread rides an opener's top-right corner as a positioned dot, so it costs
 * no width. While the chat works the dot would flicker on with every token
 * and the spinner already says "look here", so it waits for the chat to
 * settle.
 */
const UnreadBadge: FC<{ readonly chat: Chat }> = ({ chat }) => {
	if (!chat.has_unread || isActiveChatStatus(chat.status)) return null;
	return (
		<span
			role="img"
			aria-label="Unread"
			className="absolute -right-px -top-px size-1.5 rounded-full bg-content-link"
		/>
	);
};

interface ChatOpenerProps extends ChatOpeners {
	readonly chat: Chat;
}

/** The chat icon: resting on it previews the chat, clicking it pins the window. */
export const ChatOpener: FC<ChatOpenerProps> = ({
	chat,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => (
	<IconButton
		aria-label={`Open ${chat.title}`}
		title="Open chat"
		onPointerEnter={() => onPreview(chat)}
		onPointerLeave={onPreviewEnd}
		onClick={() => onOpen(chat)}
	>
		<MessageSquareIcon className="size-3.5" />
		<UnreadBadge chat={chat} />
	</IconButton>
);

interface AssistantOpenerProps extends ChatOpeners {
	readonly assistant: Chat;
}

/**
 * The card's assistant chat, which the board list hides: this icon is the
 * only sign it exists. Same preview and open gestures as ChatOpener; the
 * glyph pulses while the assistant is on a turn.
 */
export const AssistantOpener: FC<AssistantOpenerProps> = ({
	assistant,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	let label = "Assistant";
	if (assistant.status === "running") label = "Assistant working";
	else if (assistant.status === "interrupting") label = "Assistant stopping";
	return (
		<IconButton
			aria-label={label}
			title={label}
			onPointerEnter={() => onPreview(assistant)}
			onPointerLeave={onPreviewEnd}
			onClick={() => onOpen(assistant)}
		>
			<BotIcon
				className={cn(
					"size-3.5",
					isActiveChatStatus(assistant.status) && "animate-pulse",
				)}
			/>
			<UnreadBadge chat={assistant} />
		</IconButton>
	);
};
