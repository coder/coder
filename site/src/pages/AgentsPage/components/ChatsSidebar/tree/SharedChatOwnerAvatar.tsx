import { cn } from "cn";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";

type SharedChatOwnerAvatarProps = {
	readonly chat: Chat;
	/** Accessible description of the chat status, e.g. "Working". */
	readonly statusLabel: string;
	readonly showUnread: boolean;
	readonly "data-testid"?: string;
};

/**
 * Stands in for the status icon on chats shared with the current user.
 * A running chat shimmers the avatar and an unread chat shows a dot
 * on the avatar's corner.
 */
export const SharedChatOwnerAvatar: FC<SharedChatOwnerAvatarProps> = ({
	chat,
	statusLabel,
	showUnread,
	"data-testid": testId,
}) => {
	const ownerLabel = chat.owner_name || chat.owner_username || "Unknown user";
	const isRunning = chat.status === "running";
	// Avatar pads emoji inline for its larger sizes, which leaves the emoji
	// tiny in this 18px circle; the important modifier overrides that.
	const isEmojiAvatar = chat.owner_avatar_url?.startsWith("/emojis/");

	return (
		<span
			role="img"
			aria-label={`Shared by ${ownerLabel}, ${statusLabel}`}
			data-testid={testId}
			className="relative inline-flex size-4.5 shrink-0"
		>
			<Avatar
				size="sm"
				src={chat.owner_avatar_url}
				fallback={chat.owner_username || chat.owner_name}
				className={cn(
					"size-4.5 rounded-full text-[8px]",
					isEmojiAvatar && "p-0.5!",
				)}
			>
				{isRunning && (
					<span
						data-testid={`shared-chat-avatar-shimmer-${chat.id}`}
						aria-hidden="true"
						className={cn(
							"pointer-events-none absolute inset-0",
							"bg-linear-to-r from-transparent via-white/60 to-transparent",
							"animate-avatar-shimmer motion-reduce:animate-none motion-reduce:bg-white/20",
						)}
					/>
				)}
			</Avatar>
			{showUnread && (
				<span
					data-testid={`unread-indicator-${chat.id}`}
					aria-hidden="true"
					className="absolute -top-0.5 -right-0.5 size-2 rounded-full bg-content-link ring-2 ring-surface-primary"
				/>
			)}
		</span>
	);
};
