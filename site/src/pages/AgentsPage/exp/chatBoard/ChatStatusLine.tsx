import { cn } from "cn";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { shortRelativeTime } from "#/utils/time";
import { isActiveChatStatus } from "../../components/ChatConversation/chatStore";
import { getChatDisplayConfig } from "../../components/ChatsSidebar/tree/statusConfig";

interface ChatStatusLineProps {
	readonly chat: Chat;
	/** Placement in the card grid; the line renders nothing when there is nothing to say. */
	readonly className?: string;
}

/**
 * One line under a chat's title: PR chip, last turn text, then the unread
 * mark and the age at the right edge, since the age is the time of that
 * turn. While the chat works both would read "now" and say nothing, so they
 * are omitted until it settles. Shared by single cards and group rows.
 */
export const ChatStatusLine: FC<ChatStatusLineProps> = ({
	chat,
	className,
}) => {
	const display = getChatDisplayConfig(chat);
	const pr = display.diffStatus;
	const settled = !isActiveChatStatus(chat.status);
	if (!chat.last_turn_summary && !pr?.url && !settled) return null;
	return (
		<div
			className={cn(
				"flex min-w-0 items-center gap-x-1.5 text-xs leading-4 text-content-secondary",
				className,
			)}
		>
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
			{chat.last_turn_summary && (
				<span className="min-w-0 flex-1 truncate">
					{chat.last_turn_summary}
				</span>
			)}
			{settled && (
				// The ⋮ glyph above ends ~6px inside its button; the age lines up
				// with the glyph, not the box.
				<span className="ml-auto flex shrink-0 items-center gap-1.5 pr-1.5">
					{chat.has_unread && (
						<span
							role="img"
							className="size-[7px] shrink-0 rounded-full bg-content-link"
							aria-label="Unread"
						/>
					)}
					<span className="text-[11px] tabular-nums text-content-secondary/70">
						{shortRelativeTime(chat.updated_at)}
					</span>
				</span>
			)}
		</div>
	);
};
