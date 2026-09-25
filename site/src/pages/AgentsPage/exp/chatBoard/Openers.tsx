import { cn } from "cn";
import { BotIcon, MessageSquareIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import { IconButton } from "./IconButton";

/** Inside a card or row the anchor is fixed, so its openers pass only the chat. */
type ChatOpeners = {
	readonly onOpen: (chat: Chat) => void;
	readonly onPreview: (chat: Chat) => void;
	readonly onPreviewEnd: () => void;
};

/**
 * Hidden while the chat works: the spinner already marks the chat, and the
 * dot would flicker on each token.
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

type ChatOpenerProps = {
	readonly chat: Chat;
} & ChatOpeners;

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

type AssistantOpenerProps = {
	readonly assistant: Chat;
} & ChatOpeners;

// Covers every status isActiveChatStatus treats as active: all of them
// pulse, so only the label tells them apart.
const assistantLabel: Partial<Record<Chat["status"], string>> = {
	running: "Assistant working",
	requires_action: "Assistant waiting for you",
	interrupting: "Assistant stopping",
};

/** The board list hides assistant chats; this icon is the only sign a card has one. */
export const AssistantOpener: FC<AssistantOpenerProps> = ({
	assistant,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	const label = assistantLabel[assistant.status] ?? "Assistant";
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
