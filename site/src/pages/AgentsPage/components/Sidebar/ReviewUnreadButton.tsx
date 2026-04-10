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
	const { unreadCount, hasReviewThreshold } = useUnreadChats(chatList);

	if (!hasReviewThreshold) {
		return null;
	}

	return (
		<button
			type="button"
			className={cn(
				"relative flex w-full items-center gap-2.5 rounded-md px-2.5 py-2",
				"text-left text-sm cursor-pointer transition-all",
				"bg-surface-orange/30 text-content-warning",
				"shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				"hover:bg-surface-orange/50",
			)}
			onClick={onClick}
		>
			<ClipboardListIcon className="size-4 shrink-0" />
			<span className="truncate font-medium">Review unread chats</span>
			<span className="absolute -top-1.5 -right-1.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-content-warning px-1 text-xs font-bold text-white">
				{unreadCount}
			</span>
		</button>
	);
};
