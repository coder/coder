import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { chatCost, chat as chatQuery } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { getChatCostTreeID } from "../../components/ChatConversation/chatHelpers";
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
export const ChatInfoPopover: FC<ChatInfoPopoverProps> = ({ chat }) => {
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
				<button
					type="button"
					aria-label={`Details for ${chat.title}`}
					// Above the row's open-chat link, so this click stays its own.
					className={cn(
						"relative z-[1] grid size-4 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:text-content-primary",
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
					<InfoIcon className="size-3.5" />
				</button>
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
const ChatInfoBody: FC<ChatInfoBodyProps> = ({ chat }) => {
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
