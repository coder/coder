import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useQuery } from "react-query";
import { chatCost, chat as chatQuery } from "#/api/queries/chats";
import type { Chat, ChatDiffStatus } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { getChatCostTreeID } from "../../components/ChatConversation/chatHelpers";
import {
	getChatDisplayConfig,
	getPRIconConfig,
} from "../../components/ChatsSidebar/tree/statusConfig";
import { originRepoLabel } from "../../utils/originRepoLabel";
import { prNumber } from "../../utils/pullRequest";
import { CompactMarkdown } from "./CompactMarkdown";

type ChatInfoPopoverProps = {
	readonly chat: Chat;
};

// Long enough that crossing the icon does not flash the popover; the close
// delay lets the pointer travel from the icon into the content.
const HOVER_OPEN_MS = 300;
const HOVER_CLOSE_MS = 200;

/**
 * The Summary tab's data next to a chat row. Hover previews it after a short
 * delay and it stays while the pointer is on trigger or content; a click
 * pins it, a second click closes it. Portaled, so it never pushes card
 * content around or clips. The click handler prevents default so Radix's
 * own open toggle stays out of it; Radix still keeps a click on the trigger
 * from counting as an outside interaction.
 */
export const ChatInfoPopover: React.FC<ChatInfoPopoverProps> = ({ chat }) => {
	const [state, setState] = useState<"closed" | "hover" | "pinned">("closed");
	// The hover transition waiting for its delay. Pointer moves replace it,
	// so crossing the icon never opens and a quick return never closes.
	const [pending, setPending] = useState<"open" | "close" | null>(null);
	useEffect(() => {
		if (!pending) return;
		const timer = setTimeout(
			() => {
				setPending(null);
				setState((s) => {
					if (pending === "open") return s === "closed" ? "hover" : s;
					return s === "hover" ? "closed" : s;
				});
			},
			pending === "open" ? HOVER_OPEN_MS : HOVER_CLOSE_MS,
		);
		return () => clearTimeout(timer);
	}, [pending]);
	const hoverIn = () => setPending("open");
	const hoverOut = () => setPending("close");
	const open = state !== "closed";

	return (
		<Popover
			open={open}
			onOpenChange={(next) => {
				if (!next) setState("closed");
			}}
		>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={`Details for ${chat.title}`}
					// Above the row's open-chat link, so this click stays its own.
					className={cn(
						"relative z-[1] size-4 min-w-0 rounded p-0 text-content-secondary/60 [&>svg]:size-3.5! [&>svg]:p-0",
						state === "pinned" && "text-content-primary",
					)}
					onPointerEnter={hoverIn}
					onPointerLeave={hoverOut}
					onPointerDown={(e) => e.stopPropagation()}
					onClick={(e) => {
						e.preventDefault();
						setPending(null);
						setState((s) => (s === "pinned" ? "closed" : "pinned"));
					}}
				>
					<InfoIcon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				side="right"
				align="start"
				sideOffset={8}
				collisionPadding={12}
				className="w-[300px] p-0"
				onPointerEnter={hoverIn}
				onPointerLeave={hoverOut}
				onPointerDown={(e) => e.stopPropagation()}
				// A hover preview must not steal focus from what the user is doing.
				onOpenAutoFocus={(e) => {
					if (state !== "pinned") e.preventDefault();
				}}
			>
				{open && <ChatInfoBody chat={chat} />}
			</PopoverContent>
		</Popover>
	);
};

type ChatInfoBodyProps = {
	readonly chat: Chat;
};

/** Fetches only while mounted, so closed popovers cost nothing. */
const ChatInfoBody: React.FC<ChatInfoBodyProps> = ({ chat }) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const detail = useQuery(chatQuery(chat.id)).data;
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(detail) ?? chat.id),
		enabled: showCost && detail !== undefined,
	});
	const summary = (detail?.summary ?? chat.summary)?.trim();
	const noSummary = chat.parent_chat_id
		? "Summary pending agent completion."
		: "No summary yet.";
	let cost = "...";
	if (costQuery.data) cost = formatCostMicros(costQuery.data.total_cost_micros);
	else if (costQuery.isError) cost = "unavailable";
	const unpricedRequests = costQuery.data?.unpriced_request_count ?? 0;
	const prs = getChatDisplayConfig(detail ?? chat).prStatuses.filter(
		(status) => status.url,
	);

	return (
		<div className="max-h-[60vh] overflow-y-auto px-3.5 py-3 text-[12.5px] leading-[1.45] text-content-primary">
			<div className="mb-2 text-[13px] font-medium text-pretty">
				{chat.title}
			</div>
			{summary ? (
				<CompactMarkdown className="text-[12.5px] leading-[1.45] text-content-primary/85">
					{summary}
				</CompactMarkdown>
			) : (
				<span className="text-content-secondary">{noSummary}</span>
			)}
			<dl className="m-0 mt-2.5 grid grid-cols-[auto_1fr] gap-x-3.5 gap-y-1 border-t border-border pt-2.5 text-xs text-content-secondary">
				<dt>Created</dt>
				<dd className="m-0 text-content-primary">
					{formatDateTime(chat.created_at, DATE_FORMAT.MEDIUM_DATE)}
				</dd>
				<dt>Updated</dt>
				<dd className="m-0 text-content-primary">
					{formatDateTime(chat.updated_at, DATE_FORMAT.MEDIUM_DATE)}
				</dd>
				{prs.length > 0 && (
					<>
						<dt>{prs.length === 1 ? "Changes" : `${prs.length} PRs`}</dt>
						<dd className="m-0 flex flex-col gap-0.5 font-mono text-content-primary">
							{prs.map((status) => (
								<PRRow
									key={`${status.remote_origin}/${status.git_branch}`}
									status={status}
									showRepo={new Set(prs.map((p) => p.remote_origin)).size > 1}
								/>
							))}
						</dd>
					</>
				)}
				{showCost && (
					<>
						<dt>Cost</dt>
						<dd className="m-0 font-mono text-content-primary">{cost}</dd>
					</>
				)}
			</dl>
			{showCost && costQuery.data && chat.parent_chat_id && (
				<p className="m-0 mt-1.5 text-xs italic text-content-secondary">
					Cost covers this agent's whole chat, including the chat that started
					it and any other subagents.
				</p>
			)}
			{showCost && costQuery.data && unpricedRequests > 0 && (
				<p className="m-0 mt-1.5 text-xs italic text-content-secondary">
					Excludes unpriced usage from {unpricedRequests} request
					{unpricedRequests === 1 ? "" : "s"}.
				</p>
			)}
		</div>
	);
};

type PRRowProps = {
	readonly status: ChatDiffStatus;
	/** PR numbers repeat across repositories, so name it when PRs span several. */
	readonly showRepo: boolean;
};

const PRRow: React.FC<PRRowProps> = ({ status, showRepo }) => {
	const number = prNumber(status);
	const state = getPRIconConfig(status);
	return (
		<span className="flex min-w-0 items-center gap-2">
			<a
				href={status.url}
				target="_blank"
				rel="noreferrer"
				aria-label={
					state ? `${number ? `#${number}` : "PR"}, ${state.label}` : undefined
				}
				className="inline-flex min-w-0 items-center gap-1 text-content-link no-underline hover:underline"
			>
				<span
					className={cn(
						"size-1.5 shrink-0 rounded-full bg-current",
						state?.className,
					)}
				/>
				<span className="truncate">
					{showRepo && status.remote_origin
						? `${originRepoLabel(status.remote_origin)}#${number ?? ""}`
						: number
							? `#${number}`
							: "PR"}
				</span>
			</a>
			<span className="shrink-0">
				<span className="text-git-added-bright">+{status.additions}</span>{" "}
				<span className="text-git-deleted-bright">
					&minus;{status.deletions}
				</span>
			</span>
		</span>
	);
};
