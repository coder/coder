import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { ChatTreeNode } from "./ChatTreeNode";

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
	return (
		<div className="my-0.5 flex flex-col gap-0.5 rounded-md border border-content-link/40 bg-surface-secondary/50 p-0.5">
			<ChatTreeNode chat={chat} />
			{members.map((member) => (
				<ChatTreeNode key={member.id} chat={member} />
			))}
		</div>
	);
};
