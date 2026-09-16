import { cn } from "cn";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import {
	type CardColor,
	getColorLabel,
	getColumnLabel,
	getTitleLabel,
	INBOX_COLUMN,
} from "../../ChatBoard/boardLabels";
import { ChatTreeNode } from "./ChatTreeNode";

// Group box tinted with the card's color; neutral when the card has none.
const GROUP_BOX_CLASS: Record<CardColor | "none", string> = {
	none: "border-border bg-surface-secondary/60 text-content-secondary",
	green: "border-highlight-green/50 bg-surface-green/60 text-highlight-green",
	orange:
		"border-highlight-orange/50 bg-surface-orange/60 text-highlight-orange",
	sky: "border-highlight-sky/50 bg-surface-sky/60 text-highlight-sky",
	red: "border-highlight-red/50 bg-surface-red/60 text-highlight-red",
	purple:
		"border-highlight-purple/50 bg-surface-purple/60 text-highlight-purple",
	magenta:
		"border-highlight-magenta/50 bg-surface-magenta/60 text-highlight-magenta",
};

interface BoardGroupEntryProps {
	readonly chat: Chat;
	readonly members: readonly Chat[] | undefined;
}

/**
 * One list entry: a plain node, or a box holding a board primary followed by
 * its members. Members stay top-level nodes, so nothing collapses them.
 */
export const BoardGroupEntry: FC<BoardGroupEntryProps> = ({
	chat,
	members,
}) => {
	if (!members || members.length === 0) {
		return <ChatTreeNode chat={chat} />;
	}
	const column = getColumnLabel(chat);
	return (
		<div
			className={cn(
				"my-0.5 flex flex-col gap-px rounded-lg border px-[3px] pt-0.5 pb-[3px]",
				GROUP_BOX_CLASS[getColorLabel(chat) ?? "none"],
			)}
		>
			<div className="flex items-center gap-1.5 px-2 pt-[5px] pb-0.5 text-[11px]">
				<span className="size-1.5 shrink-0 rounded-[2px] bg-current" />
				<span className="min-w-0 truncate font-medium">
					{getTitleLabel(chat) ?? chat.title}
				</span>
				{column !== INBOX_COLUMN && (
					<span className="shrink-0 font-mono text-content-secondary/70">
						· {column}
					</span>
				)}
			</div>
			{/* The header already names the column, so the rows do not repeat it. */}
			<ChatTreeNode chat={chat} showBoardColumn={false} />
			{members.map((member) => (
				<ChatTreeNode key={member.id} chat={member} showBoardColumn={false} />
			))}
		</div>
	);
};
