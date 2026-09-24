import { cn } from "cn";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { getChatDisplayConfig } from "../../components/ChatsSidebar/tree/statusConfig";

type ChatStatusLineProps = {
	readonly chat: Chat;
};

/** PR chip, line stats, and last turn text; shared by single cards and group rows. */
export const ChatStatusLine: FC<ChatStatusLineProps> = ({ chat }) => {
	const display = getChatDisplayConfig(chat);
	const pr = display.diffStatus;
	const hasLineStats =
		pr !== undefined && (pr.additions > 0 || pr.deletions > 0);
	if (!chat.last_turn_summary && !pr?.url) return null;
	return (
		<div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs leading-4 text-content-secondary">
			{pr?.url && display.prIcon && (
				<a
					href={pr.url}
					target="_blank"
					rel="noreferrer"
					aria-label={display.prIcon.label}
					className="relative z-[1] inline-flex h-4 shrink-0 items-center gap-1 rounded bg-content-primary/5 px-1.5 font-mono text-[11px] text-content-secondary no-underline hover:text-content-primary"
					onPointerDown={(e) => e.stopPropagation()}
				>
					<span
						className={cn(
							"size-1.5 rounded-full bg-current",
							display.prIcon.className,
						)}
					/>
					{pr.pr_number ? `#${pr.pr_number}` : "PR"}
				</a>
			)}
			{hasLineStats && (
				<span className="shrink-0 font-mono text-[11px]">
					<span className="text-git-added-bright">+{pr.additions}</span>{" "}
					<span className="text-git-deleted-bright">&minus;{pr.deletions}</span>
				</span>
			)}
			{chat.last_turn_summary && (
				<span className="min-w-0 flex-1 truncate">
					{chat.last_turn_summary}
				</span>
			)}
		</div>
	);
};
