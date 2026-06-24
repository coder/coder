import { InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { chat, chatCost } from "#/api/queries/chats";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { ChatSummary } from "./ChatSummary";

type ChatSummaryButtonProps = {
	chatId: string;
};

type ChatSummaryPopoverContentProps = ChatSummaryButtonProps & {
	open: boolean;
};

/**
 * ChatSummaryPopoverContent is the data wrapper for the summary popover. It
 * owns the React Query reads so the ChatSummary component stays presentational.
 */
export const ChatSummaryPopoverContent: FC<ChatSummaryPopoverContentProps> = ({
	chatId,
	open,
}) => {
	// Created/Updated come from the cached chat query, which the AgentsPage
	// keeps live via its watchChats merge, so this adds no extra request.
	const chatQuery = useQuery({ ...chat(chatId), enabled: open });
	// Cost is dynamic and not part of the cached chat, so it is fetched on
	// demand only while the popover is open.
	const costQuery = useQuery({ ...chatCost(chatId), enabled: open });

	const chatData = chatQuery.data;

	return (
		<PopoverContent
			align="end"
			className="w-[calc(100vw-2rem)] p-3 sm:w-[400px] sm:p-4"
		>
			{chatData ? (
				<ChatSummary
					// The persisted whole-chat summary is generated asynchronously
					// and may be null until the first summary is produced; the
					// component renders a muted empty state in that case.
					summary={chatData.summary}
					createdAt={chatData.created_at}
					updatedAt={chatData.updated_at}
					costMicros={costQuery.data?.total_cost_micros}
					isCostLoading={costQuery.isLoading}
				/>
			) : (
				<div
					role="status"
					className="flex items-center justify-center py-8 text-content-secondary"
				>
					<Spinner loading />
				</div>
			)}
		</PopoverContent>
	);
};

/**
 * ChatSummaryButton is a self-contained info button that opens the summary
 * popover. The chat header uses a render-prop variant (see ChatTopBar), but
 * this bundled version is convenient for reuse and Storybook.
 */
export const ChatSummaryButton: FC<ChatSummaryButtonProps> = ({ chatId }) => {
	const [open, setOpen] = useState(false);
	const [contentGeneration, setContentGeneration] = useState(0);

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			setContentGeneration((generation) => generation + 1);
		}

		setOpen(nextOpen);
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					className="size-7 text-content-secondary hover:text-content-primary"
					aria-label="Show chat summary"
				>
					<InfoIcon className="size-4" />
				</Button>
			</PopoverTrigger>
			<ChatSummaryPopoverContent
				key={contentGeneration}
				chatId={chatId}
				open={open}
			/>
		</Popover>
	);
};
