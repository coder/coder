import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import { useQuery } from "react-query";
import { chatCost, chat as chatQuery } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { Markdown } from "#/components/Markdown/Markdown";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { getChatCostTreeID } from "../../components/ChatConversation/chatHelpers";
import { COMPACT_MARKDOWN_CLASS } from "./NotesSection";

interface ChatInfoPopoverProps {
	readonly chat: Chat;
}

/**
 * The Summary tab's data next to a chat row. Hover previews it after a short
 * delay and it stays while the pointer is on trigger or content; a click
 * pins it, a second click closes it. Portaled, so it never pushes card
 * content around or clips. The button is an anchor, not a Radix trigger,
 * because the trigger's own click toggle would fight the hover state.
 */
export const ChatInfoPopover: FC<ChatInfoPopoverProps> = ({ chat }) => {
	const [state, setState] = useState<"closed" | "hover" | "pinned">("closed");
	const button = useRef<HTMLButtonElement>(null);
	const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
	const later = (fn: () => void, ms: number) => {
		if (timer.current) clearTimeout(timer.current);
		timer.current = setTimeout(fn, ms);
	};
	const hoverIn = () =>
		later(() => setState((s) => (s === "closed" ? "hover" : s)), 300);
	const hoverOut = () =>
		later(() => setState((s) => (s === "hover" ? "closed" : s)), 200);
	const open = state !== "closed";

	return (
		<Popover
			open={open}
			onOpenChange={(next) => {
				if (!next) setState("closed");
			}}
		>
			<PopoverAnchor asChild>
				<button
					ref={button}
					type="button"
					aria-label={`Details for ${chat.title}`}
					aria-expanded={open}
					// Above the row's open-chat link, so this click stays its own.
					className={cn(
						"relative z-[1] grid size-4 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:text-content-primary",
						state === "pinned" && "text-content-primary",
					)}
					onPointerEnter={hoverIn}
					onPointerLeave={hoverOut}
					onPointerDown={(e) => e.stopPropagation()}
					onClick={() => {
						if (timer.current) clearTimeout(timer.current);
						setState((s) => (s === "pinned" ? "closed" : "pinned"));
					}}
				>
					<InfoIcon className="size-3.5" />
				</button>
			</PopoverAnchor>
			<PopoverContent
				side="right"
				align="start"
				sideOffset={8}
				collisionPadding={12}
				className="w-[300px] p-0"
				onPointerEnter={hoverIn}
				onPointerLeave={hoverOut}
				onPointerDown={(e) => e.stopPropagation()}
				// The anchor is outside the content; its clicks are handled above.
				onInteractOutside={(e) => {
					if (button.current?.contains(e.target as Node)) e.preventDefault();
				}}
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

/** Fetches only while mounted, so closed popovers cost nothing. */
const ChatInfoBody: FC<{ readonly chat: Chat }> = ({ chat }) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const detail = useQuery(chatQuery(chat.id)).data;
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(detail) ?? chat.id),
		enabled: showCost && detail !== undefined,
	});
	const summary = (detail?.summary ?? chat.summary)?.trim();

	return (
		<div className="max-h-[60vh] overflow-y-auto px-3.5 py-3 text-[12.5px] leading-[1.45] text-content-primary">
			<div className="mb-2 text-[13px] font-medium [text-wrap:pretty]">
				{chat.title}
			</div>
			{summary ? (
				<Markdown
					className={cn(
						"text-[12.5px] leading-[1.45] text-content-primary/85",
						COMPACT_MARKDOWN_CLASS,
					)}
				>
					{summary}
				</Markdown>
			) : (
				<span className="text-content-secondary">
					{chat.parent_chat_id
						? "Summary pending agent completion."
						: "No summary yet."}
				</span>
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
						<dd className="m-0 font-mono text-content-primary">
							{costQuery.data
								? formatCostMicros(costQuery.data.total_cost_micros)
								: costQuery.isError
									? "unavailable"
									: "..."}
						</dd>
					</>
				)}
			</dl>
		</div>
	);
};
