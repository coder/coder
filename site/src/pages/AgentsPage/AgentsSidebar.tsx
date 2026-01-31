import type { FC } from "react";
import type { Chat } from "api/typesGenerated";
import { formatDistanceToNow } from "date-fns";
import { cn } from "utils/cn";

interface AgentsSidebarProps {
	chats: readonly Chat[];
	selectedChatId?: string;
	onSelect: (chatId: string) => void;
}

export const AgentsSidebar: FC<AgentsSidebarProps> = ({
	chats,
	selectedChatId,
	onSelect,
}) => {
	if (chats.length === 0) {
		return null;
	}

	return (
		<nav className="w-64 shrink-0 border border-border-default rounded-lg bg-surface-primary max-h-[calc(100vh-200px)] overflow-y-auto">
			<ul className="p-2 space-y-1">
				{chats.map((chat) => (
					<li key={chat.id}>
						<button
							type="button"
							onClick={() => onSelect(chat.id)}
							className={cn(
								"w-full text-left px-3 py-2 rounded-md transition-colors",
								"hover:bg-surface-secondary",
								chat.id === selectedChatId &&
									"bg-surface-secondary font-medium",
							)}
						>
							<div className="text-sm truncate">{chat.title}</div>
							<div className="text-xs text-content-secondary mt-0.5">
								{formatDistanceToNow(new Date(chat.updated_at), {
									addSuffix: true,
								})}
							</div>
						</button>
					</li>
				))}
			</ul>
		</nav>
	);
};
