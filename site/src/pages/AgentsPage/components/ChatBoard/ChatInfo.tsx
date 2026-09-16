import { cn } from "cn";
import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { chatCost, chat as chatQuery } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { InlineMarkdown } from "#/components/Markdown/InlineMarkdown";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { formatCostMicros } from "#/utils/currency";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { getChatCostTreeID } from "../ChatConversation/chatHelpers";

interface ChatInfoButtonProps {
	readonly chat: Chat;
	readonly open: boolean;
	readonly onHover: () => void;
	readonly onToggle: () => void;
}

/** The (i) at the end of a chat row. Hover previews, click pins. */
export const ChatInfoButton: FC<ChatInfoButtonProps> = ({
	chat,
	open,
	onHover,
	onToggle,
}) => (
	<button
		type="button"
		aria-label={`Details for ${chat.title}`}
		aria-expanded={open}
		className={cn(
			"grid size-4 place-items-center rounded border-0 bg-transparent p-0 text-content-secondary/60 hover:bg-content-link/10 hover:text-content-link",
			open && "text-content-link",
		)}
		onPointerEnter={onHover}
		onPointerDown={(e) => e.stopPropagation()}
		onClick={onToggle}
	>
		<InfoIcon className="size-3.5" />
	</button>
);

interface ChatInfoPanelProps {
	readonly chat: Chat;
}

/** The Summary tab's data, inline under a chat row. Fetches only while shown. */
export const ChatInfoPanel: FC<ChatInfoPanelProps> = ({ chat }) => {
	const showCost = Boolean(useFeatureVisibility().aibridge);
	const detailQuery = useQuery(chatQuery(chat.id));
	const detail = detailQuery.data;
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(detail) ?? chat.id),
		enabled: showCost && detail !== undefined,
	});
	const summary = (detail?.summary ?? chat.summary)?.trim();

	return (
		<div className="col-span-full mt-1.5 mb-0.5 cursor-default rounded-md border border-border bg-surface-secondary px-3 py-2.5 text-[13px] leading-snug text-content-primary">
			{summary ? (
				<InlineMarkdown className="wrap-anywhere">{summary}</InlineMarkdown>
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
