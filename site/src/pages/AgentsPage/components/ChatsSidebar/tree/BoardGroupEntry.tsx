import { CopyIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import {
	getColumnLabel,
	getTitleLabel,
	INBOX_COLUMN,
} from "../../ChatBoard/boardLabels";
import { ChatTreeNode } from "./ChatTreeNode";
import { ColumnTag } from "./ColumnTag";

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
		// An inset ring instead of border plus padding keeps rows inside on
		// the same x as rows outside.
		<div className="my-0.5 flex flex-col gap-px rounded-lg bg-surface-secondary/60 ring-1 ring-border ring-inset">
			{/* The column belongs to the card, so it shows once here and never on the rows. */}
			<div className="grid grid-cols-[14px_minmax(0,1fr)_auto] items-center gap-x-2 px-2 pt-1.5 pb-[3px] text-xs">
				<CopyIcon
					className="size-[13px] text-content-secondary"
					aria-label={`Group of ${members.length + 1} chats`}
				/>
				<span className="min-w-0 truncate font-medium text-content-primary">
					{getTitleLabel(chat) ?? chat.title}
				</span>
				{column !== INBOX_COLUMN && <ColumnTag name={column} />}
			</div>
			<ChatTreeNode chat={chat} showBoardColumn={false} />
			{members.map((member) => (
				<ChatTreeNode key={member.id} chat={member} showBoardColumn={false} />
			))}
		</div>
	);
};
