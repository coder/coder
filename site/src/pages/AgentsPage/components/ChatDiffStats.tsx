import { cn } from "cn";
import type { Chat, ChatDiffStatus } from "#/api/typesGenerated";
import { ChatNodePRIcon } from "./ChatsSidebar/tree/ChatNodePRIcon";
import {
	getChatDisplayConfig,
	getPRIconConfig,
} from "./ChatsSidebar/tree/statusConfig";

type ChatDiffStatsProps = {
	readonly chat: Chat;
};

export const ChatPRStateIcon: React.FC<ChatDiffStatsProps> = ({ chat }) => {
	const prIcon = getPRIconConfig(chat.diff_status);
	if (!prIcon) {
		return null;
	}
	const PRIcon = prIcon.icon;
	return (
		<PRIcon
			role="img"
			aria-label={prIcon.label}
			className={cn("size-3.5 shrink-0", prIcon.className)}
		/>
	);
};

const ChatLineStats: React.FC<{ readonly status: ChatDiffStatus }> = ({
	status,
}) => {
	const { changed_files: changedFiles, additions, deletions } = status;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	if (!status.url || !hasLineStats) {
		return null;
	}
	const filesChangedLabel = `${changedFiles} ${
		changedFiles === 1 ? "file" : "files"
	}`;

	return (
		<span
			className="inline-flex shrink-0 items-center gap-0.5 text-[13px] leading-4 tabular-nums"
			title={`${filesChangedLabel}, +${additions} -${deletions}`}
		>
			<span className="text-git-added-bright">+{additions}</span>
			<span className="text-git-deleted-bright">&minus;{deletions}</span>
		</span>
	);
};

export const ChatDiffStats: React.FC<ChatDiffStatsProps> = ({ chat }) => {
	const { prStatuses } = getChatDisplayConfig(chat);

	// The sole PR's line stats can differ from the primary row's,
	// which may be a newer branch-only ref with zeroed counts.
	const solePR = prStatuses.length === 1 ? prStatuses[0] : undefined;

	return (
		<>
			<ChatNodePRIcon prStatuses={prStatuses} />
			{solePR && <ChatLineStats status={solePR} />}
		</>
	);
};
