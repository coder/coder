import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import { getChatDisplayConfig } from "./ChatsSidebar/tree/statusConfig";

type ChatDiffStatsProps = {
	readonly chat: Chat;
};

export const ChatPRStateIcon: React.FC<ChatDiffStatsProps> = ({ chat }) => {
	const { prIcon } = getChatDisplayConfig(chat);
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

export const ChatLineStats: React.FC<ChatDiffStatsProps> = ({ chat }) => {
	const { diffStatus } = getChatDisplayConfig(chat);
	const changedFiles = diffStatus?.changed_files ?? 0;
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	if (!diffStatus?.url || !hasLineStats) {
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

export const ChatDiffStats: React.FC<ChatDiffStatsProps> = ({ chat }) => (
	<>
		<ChatPRStateIcon chat={chat} />
		<ChatLineStats chat={chat} />
	</>
);
