import { InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chatCost, chat as chatQuery } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { getChatCostTreeID } from "../ChatConversation/chatHelpers";
import { ChatSummary } from "../ChatSummary";

interface ChatInfoPopoverProps {
	readonly chat: Chat;
}

/** The chat's Summary tab, rendered in a popover without opening the chat. */
export const ChatInfoPopover: FC<ChatInfoPopoverProps> = ({ chat }) => {
	const [open, setOpen] = useState(false);
	const showCost = Boolean(useFeatureVisibility().aibridge);
	// Both requests are per chat, so they only run once the popover opens.
	const detailQuery = useQuery({ ...chatQuery(chat.id), enabled: open });
	const detail = detailQuery.data;
	const costQuery = useQuery({
		...chatCost(getChatCostTreeID(detail) ?? chat.id),
		enabled: open && showCost && detail !== undefined,
	});

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={`Details for ${chat.title}`}
					className="size-6 shrink-0 text-content-secondary"
					onPointerDown={(e) => e.stopPropagation()}
				>
					<InfoIcon className="size-3.5" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="max-h-[70vh] w-96 overflow-y-auto p-4 text-sm"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<div className="mb-3 font-medium text-content-primary">
					{chat.title}
				</div>
				<ChatSummary
					summary={detail?.summary ?? chat.summary}
					isSubagent={Boolean(chat.parent_chat_id)}
					createdAt={chat.created_at}
					updatedAt={chat.updated_at}
					costMicros={costQuery.data?.total_cost_micros}
					unpricedRequestCount={costQuery.data?.unpriced_request_count}
					showCost={showCost}
					isCostLoading={costQuery.isLoading}
					costError={costQuery.isError}
				/>
			</PopoverContent>
		</Popover>
	);
};
