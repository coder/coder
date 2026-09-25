import { CopyIcon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { ChatTreeNode } from "../../components/ChatsSidebar/tree/ChatTreeNode";
import { type BoardGroups, isBoardGroupMember } from "./boardGroups";
import { getColumnLabel, getTitleLabel, INBOX_COLUMN } from "./boardLabels";
import { ColumnTag } from "./ColumnTag";

type BoardGroupEntryProps = {
	readonly chat: Chat;
	readonly groups: BoardGroups;
};

/**
 * One sidebar list entry: a plain node, or a box holding a board primary
 * followed by its members. Members stay top-level nodes, so nothing
 * collapses them.
 */
export const BoardGroupEntry: FC<BoardGroupEntryProps> = ({ chat, groups }) => {
	const members = groups.get(chat.id);
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
			<ChatTreeNode chat={chat} />
			{members.map((member) => (
				<ChatTreeNode key={member.id} chat={member} />
			))}
		</div>
	);
};

type BoardColumnTagProps = {
	readonly chat: Chat;
	readonly groups: BoardGroups;
};

/**
 * The trailing slot of a sidebar row: the chat's column. A group box names
 * the column once in its header, so its primary and members carry none.
 */
export const BoardColumnTag: FC<BoardColumnTagProps> = ({ chat, groups }) => {
	const column = getColumnLabel(chat);
	if (
		column === INBOX_COLUMN ||
		isBoardGroupMember(chat) ||
		groups.has(chat.id)
	) {
		return null;
	}
	return <ColumnTag name={column} />;
};
