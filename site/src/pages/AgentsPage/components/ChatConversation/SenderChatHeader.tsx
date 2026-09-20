import { ArrowLeftRightIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useLocation } from "react-router";
import type { ChatSenderChatPart } from "#/api/typesGenerated";
import { safeBuildAgentChatPath } from "../../utils/navigation";

const UNKNOWN_SENDER_CHAT_TITLE = "Unknown chat";

const RELATION_LABELS: Record<
	NonNullable<ChatSenderChatPart["sender_chat_relation"]>,
	string
> = {
	parent: "parent chat",
	child: "child chat",
};

export const senderChatDisplayTitle = (senderChat: ChatSenderChatPart) =>
	senderChat.sender_chat_title?.trim() || UNKNOWN_SENDER_CHAT_TITLE;

/**
 * Provenance line for a user row delivered by another chat's agent. A
 * missing sender id means the sender is unknown and the title is plain
 * text; a missing relay_hop means the first hop and is not shown.
 */
export const SenderChatHeader: FC<{
	readonly senderChat: ChatSenderChatPart;
}> = ({ senderChat }) => {
	const location = useLocation();
	const title = senderChatDisplayTitle(senderChat);
	const senderPath = senderChat.sender_chat_id
		? safeBuildAgentChatPath({ chatId: senderChat.sender_chat_id })
		: null;
	const relation = senderChat.sender_chat_relation
		? RELATION_LABELS[senderChat.sender_chat_relation]
		: undefined;
	const hop = senderChat.relay_hop ?? 1;

	return (
		<div
			role="group"
			aria-label={`Relayed message from ${title}`}
			className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-content-secondary"
		>
			<ArrowLeftRightIcon aria-hidden="true" className="size-3 shrink-0" />
			<span className="flex min-w-0 items-center gap-1">
				<span>From</span>
				{senderPath ? (
					<Link
						to={{ pathname: senderPath, search: location.search }}
						className="truncate font-medium text-content-link no-underline hover:underline"
					>
						{title}
					</Link>
				) : (
					<span className="truncate font-medium text-content-primary">
						{title}
					</span>
				)}
			</span>
			{relation && (
				<span className="text-content-secondary/80">{relation}</span>
			)}
			{hop > 1 && (
				<span className="rounded-sm border border-solid border-border-default px-1 tabular-nums">
					hop {hop}
				</span>
			)}
		</div>
	);
};
