import { ChevronLeftIcon, ChevronRightIcon, XIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useInfiniteQuery } from "react-query";
import { chatMessagesForInfiniteScroll } from "#/api/queries/chats";
import type { Chat, ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { deriveChatSummary } from "../hooks/useChatSummary";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import {
	buildComputerUseSubagentIds,
	buildSubagentTitles,
	parseMessagesWithMergedTools,
} from "./ChatConversation/messageParsing";

interface ReviewUnreadDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	unreadChats: Chat[];
	onChatReviewed: (chatId: string) => void;
}

export const ReviewUnreadDialog: FC<ReviewUnreadDialogProps> = ({
	open,
	onOpenChange,
	unreadChats,
	onChatReviewed,
}) => {
	const [currentIndex, setCurrentIndex] = useState(0);

	// Reset index when dialog opens or unread list changes
	useEffect(() => {
		if (open) {
			setCurrentIndex(0);
		}
	}, [open]);

	const currentChat = unreadChats[currentIndex];

	const handleNext = () => {
		if (!currentChat) return;
		onChatReviewed(currentChat.id);
		if (currentIndex >= unreadChats.length - 1) {
			onOpenChange(false);
		} else {
			setCurrentIndex((prev) => prev + 1);
		}
	};

	const handlePrevious = () => {
		if (currentIndex > 0) {
			setCurrentIndex((prev) => prev - 1);
		}
	};

	if (!currentChat) {
		return null;
	}

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="flex max-w-5xl flex-col gap-0 overflow-hidden p-0 h-[85vh]"
				aria-describedby={undefined}
			>
				{/* Header */}
				<div className="flex items-center justify-between border-b border-border-default px-6 py-4">
					<DialogTitle className="truncate text-lg">
						{currentChat.title || "Untitled chat"}
					</DialogTitle>
					<Button
						variant="subtle"
						size="icon"
						onClick={() => onOpenChange(false)}
						aria-label="Close"
					>
						<XIcon className="size-4" />
					</Button>
				</div>

				{/* Review Banner */}
				<ReviewBanner chat={currentChat} />

				{/* Chat Content */}
				<div className="flex-1 overflow-y-auto min-h-0">
					<ReviewChatContent chatId={currentChat.id} />
				</div>

				{/* Navigation Bar */}
				<ReviewNavigationBar
					currentIndex={currentIndex}
					totalCount={unreadChats.length}
					onPrevious={handlePrevious}
					onNext={handleNext}
					isLast={currentIndex >= unreadChats.length - 1}
				/>
			</DialogContent>
		</Dialog>
	);
};

interface ReviewBannerProps {
	chat: Chat;
}

const ReviewBanner: FC<ReviewBannerProps> = ({ chat }) => {
	const messagesQuery = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(chat.id),
		enabled: true,
	});

	const allMessages: ChatMessage[] =
		messagesQuery.data?.pages.flatMap((page) => page.messages).reverse() ?? [];

	const firstUserMessage = allMessages.find((m) => m.role === "user");
	const firstUserText = firstUserMessage?.content
		?.filter(
			(part): part is { type: "text"; text: string } => part.type === "text",
		)
		.map((part) => part.text)
		.join(" ");

	const summary = deriveChatSummary(chat, allMessages);

	return (
		<div className="border-b border-border-default bg-surface-secondary px-6 py-4 space-y-2">
			{firstUserText && (
				<p className="text-sm text-content-secondary line-clamp-2">
					<span className="font-medium text-content-primary">
						Original message:
					</span>{" "}
					{firstUserText}
				</p>
			)}
			{summary && <p className="text-sm text-content-secondary">{summary}</p>}
		</div>
	);
};

interface ReviewChatContentProps {
	chatId: string;
}

const ReviewChatContent: FC<ReviewChatContentProps> = ({ chatId }) => {
	const messagesQuery = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(chatId),
	});

	const allMessages: ChatMessage[] =
		messagesQuery.data?.pages.flatMap((page) => page.messages).reverse() ?? [];

	const parsedMessages = parseMessagesWithMergedTools(allMessages);
	const subagentTitles = buildSubagentTitles(parsedMessages);
	const computerUseSubagentIds = buildComputerUseSubagentIds(parsedMessages);

	if (messagesQuery.isLoading) {
		return (
			<div className="flex h-full items-center justify-center">
				<Spinner loading className="size-8 text-content-secondary" />
			</div>
		);
	}

	if (allMessages.length === 0) {
		return (
			<div className="flex h-full items-center justify-center text-sm text-content-secondary">
				No messages in this chat.
			</div>
		);
	}

	return (
		<div className="mx-auto flex w-full max-w-3xl flex-col gap-2 py-6">
			<ConversationTimeline
				parsedMessages={parsedMessages}
				subagentTitles={subagentTitles}
				computerUseSubagentIds={computerUseSubagentIds}
				showDesktopPreviews={false}
			/>
		</div>
	);
};

interface ReviewNavigationBarProps {
	currentIndex: number;
	totalCount: number;
	onPrevious: () => void;
	onNext: () => void;
	isLast: boolean;
}

const ReviewNavigationBar: FC<ReviewNavigationBarProps> = ({
	currentIndex,
	totalCount,
	onPrevious,
	onNext,
	isLast,
}) => {
	return (
		<div className="flex items-center justify-between border-t border-border-default px-6 py-3 bg-surface-primary">
			<Button
				variant="outline"
				size="sm"
				onClick={onPrevious}
				disabled={currentIndex === 0}
			>
				<ChevronLeftIcon className="size-4" />
				Previous
			</Button>

			<span className="text-sm text-content-secondary">
				{currentIndex + 1} of {totalCount}
			</span>

			<Button
				variant={isLast ? "default" : "outline"}
				size="sm"
				onClick={onNext}
			>
				{isLast ? "Done" : "Next"}
				{!isLast && <ChevronRightIcon className="size-4" />}
			</Button>
		</div>
	);
};
