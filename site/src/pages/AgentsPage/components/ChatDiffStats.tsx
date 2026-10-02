import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import { getChatDisplayConfig } from "./ChatsSidebar/tree/statusConfig";

type ChatDiffStatsProps = {
	readonly chat: Chat;
};

/**
 * The chat's pull request state icon and its `+additions -deletions` line
 * stats. Renders inline fragments so the caller controls spacing.
 */
export const ChatDiffStats: React.FC<ChatDiffStatsProps> = ({ chat }) => {
	const { prIcon, diffStatus } = getChatDisplayConfig(chat);
	const PRIcon = prIcon?.icon;
	const hasLinkedDiffStatus = Boolean(diffStatus?.url);
	const changedFiles = diffStatus?.changed_files ?? 0;
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	const hasLineStats = additions > 0 || deletions > 0 || changedFiles > 0;
	const filesChangedLabel = `${changedFiles} ${
		changedFiles === 1 ? "file" : "files"
	}`;

	return (
		<>
			{PRIcon && prIcon && (
				<PRIcon
					role="img"
					aria-label={prIcon.label}
					className={cn("size-3.5 shrink-0", prIcon.className)}
				/>
			)}
			{hasLinkedDiffStatus && hasLineStats && (
				<span
					className="inline-flex shrink-0 items-center gap-0.5 text-[13px] leading-4 tabular-nums"
					title={`${filesChangedLabel}, +${additions} -${deletions}`}
				>
					<span className="text-git-added-bright">+{additions}</span>
					<span className="text-git-deleted-bright">&minus;{deletions}</span>
				</span>
			)}
		</>
	);
};
