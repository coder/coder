import { cn } from "cn";
import { BotIcon, MessageSquareIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";

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

type ChatOpenerProps = ChatOpeners & {
	readonly chat: Chat;
};

/** The chat icon: resting on it previews the chat, clicking it pins the window. */
export const ChatOpener: FC<ChatOpenerProps> = ({
	chat,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => (
	<Button
		variant="subtle"
		size="icon"
		aria-label={`Open ${chat.title}`}
		title="Open chat"
		className="relative z-[1] size-4 min-w-0 rounded p-0 text-content-secondary/60 [&>svg]:size-3.5! [&>svg]:p-0"
		// The header is a drag handle and a card's open-chat surface lies
		// under it: the press must not start a drag, and z-[1] keeps it on top.
		onPointerDown={(e) => e.stopPropagation()}
		onPointerEnter={() => onPreview(chat)}
		onPointerLeave={onPreviewEnd}
		onClick={() => onOpen(chat)}
	>
		<MessageSquareIcon />
		<UnreadBadge chat={chat} />
	</Button>
);

type AssistantOpenerProps = ChatOpeners & {
	readonly assistant: Chat;
};

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
		<Button
			variant="subtle"
			size="icon"
			aria-label={label}
			title={label}
			className="relative z-[1] size-4 min-w-0 rounded p-0 text-content-secondary/60 [&>svg]:size-3.5! [&>svg]:p-0"
			// The header is a drag handle and a card's open-chat surface lies
			// under it: the press must not start a drag, and z-[1] keeps it on top.
			onPointerDown={(e) => e.stopPropagation()}
			onPointerEnter={() => onPreview(assistant)}
			onPointerLeave={onPreviewEnd}
			onClick={() => onOpen(assistant)}
		>
			<BotIcon
				className={cn(isActiveChatStatus(assistant.status) && "animate-pulse")}
			/>
			<UnreadBadge chat={assistant} />
		</Button>
	);
};
