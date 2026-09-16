import { InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chatCost } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { formatCostMicros } from "#/utils/currency";

interface ChatInfoPopoverProps {
	readonly chat: Chat;
}

/** Same data as the chat's Summary tab, without opening the chat. */
export const ChatInfoPopover: FC<ChatInfoPopoverProps> = ({ chat }) => {
	const [open, setOpen] = useState(false);
	// Cost is one request per chat, so only fetch it once the popover opens.
	const costQuery = useQuery({
		...chatCost(chat.root_chat_id ?? chat.id),
		enabled: open,
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
				className="w-80 space-y-2 p-3 text-sm"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<div className="font-medium text-content-primary">{chat.title}</div>
				<p className="m-0 whitespace-pre-wrap text-content-secondary">
					{chat.summary ?? chat.last_turn_summary ?? "No summary yet."}
				</p>
				<div className="flex justify-between text-xs text-content-secondary">
					<span>
						Cost:{" "}
						{costQuery.data
							? formatCostMicros(costQuery.data.total_cost_micros)
							: costQuery.isError
								? "unavailable"
								: "..."}
					</span>
					<span>{new Date(chat.created_at).toLocaleDateString("en-US")}</span>
				</div>
			</PopoverContent>
		</Popover>
	);
};
