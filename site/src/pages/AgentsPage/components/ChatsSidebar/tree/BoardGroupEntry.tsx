import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { getColumnLabel, INBOX_COLUMN } from "../../ChatBoard/boardLabels";
import { ChatTreeNode } from "./ChatTreeNode";

interface BoardGroupEntryProps {
	readonly chat: Chat;
	readonly members: readonly Chat[] | undefined;
}

const TITLE_KEY = "board/title";

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
		<div className="my-0.5 flex flex-col gap-px rounded-lg border border-content-link/45 bg-content-link/5 px-[3px] pt-0.5 pb-[3px]">
			<div className="flex items-center gap-1.5 px-2 pt-[5px] pb-0.5 text-[11px] text-content-link">
				<span className="size-1.5 shrink-0 rounded-[2px] bg-content-link" />
				<span className="min-w-0 truncate font-medium">
					{chat.labels[TITLE_KEY] ?? chat.title}
				</span>
				{column !== INBOX_COLUMN && (
					<span className="shrink-0 font-mono text-content-secondary/70">
						· {column}
					</span>
				)}
			</div>
			<ChatTreeNode chat={chat} />
			{members.map((member) => (
				<ChatTreeNode key={member.id} chat={member} />
			))}
		</div>
	);
};
