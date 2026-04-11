import { ClipboardListIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { useUnreadChats } from "../../hooks/useUnreadChats";

interface ReviewUnreadButtonProps {
	chatList: readonly Chat[];
	onClick: () => void;
}

export const ReviewUnreadButton: FC<ReviewUnreadButtonProps> = ({
	chatList,
	onClick,
}) => {
	const { unreadCount } = useUnreadChats(chatList);
	const hasUnread = unreadCount > 0;

	return (
		<button
			type="button"
			className={cn(
				"relative mt-1.5 flex w-full items-center gap-2.5 rounded-md border-0 px-2.5 py-2",
				"text-left text-sm cursor-pointer transition-colors no-underline",
				"bg-transparent text-content-secondary",
				"hover:bg-surface-tertiary/50 hover:text-content-primary",
				hasUnread && "shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
			)}
			onClick={onClick}
		>
			<ClipboardListIcon className="h-4 w-4 shrink-0" />
			<span className="truncate">Review unread chats</span>
			{hasUnread && (
				<span className="absolute -top-1.5 -right-1.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-content-warning px-1 text-xs font-bold text-white">
					{unreadCount}
				</span>
			)}
		</button>
	);
};
