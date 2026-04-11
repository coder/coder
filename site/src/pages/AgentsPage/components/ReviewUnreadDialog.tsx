import {
	ArrowUpIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	ExternalLinkIcon,
	XIcon,
} from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { useInfiniteQuery } from "react-query";
import { useNavigate } from "react-router";
import { chatMessagesForInfiniteScroll } from "#/api/queries/chats";
import type { Chat, ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
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
	const navigate = useNavigate();

	// Snapshot the unread chat list when the dialog opens so that
	// marking chats as read doesn't shift indices mid-review.
	const [snapshotChats, setSnapshotChats] = useState<Chat[]>([]);

	// Capture a stable snapshot when the dialog opens and reset
	// the index so every review session starts from the first
	// chat. We intentionally omit unreadChats from the deps so
	// the snapshot stays frozen for the duration of the session.
	// biome-ignore lint/correctness/useExhaustiveDependencies: snapshot only on open
	useEffect(() => {
		if (open) {
			setSnapshotChats(unreadChats);
			setCurrentIndex(0);
		}
	}, [open]);

	const currentChat = snapshotChats[currentIndex];

	const handleNext = () => {
		if (!currentChat) return;
		onChatReviewed(currentChat.id);
		if (currentIndex >= snapshotChats.length - 1) {
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

	const handleOpenInChat = () => {
		if (!currentChat) return;
		onOpenChange(false);
		navigate(`/agents/${currentChat.id}`);
	};

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent
				className="flex max-w-5xl flex-col gap-0 overflow-hidden p-0 h-[85vh]"
				aria-describedby={undefined}
			>
				{currentChat ? (
					<>
						{/* Header */}
						<div className="flex items-center justify-between border-b border-border-default px-6 py-4">
							<DialogTitle className="truncate text-lg">
								{currentChat.title || "Untitled chat"}
							</DialogTitle>
							<div className="flex items-center gap-1 shrink-0">
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											variant="subtle"
											size="icon"
											onClick={handleOpenInChat}
											aria-label="Open in chat"
										>
											<ExternalLinkIcon className="size-4" />
										</Button>
									</TooltipTrigger>
									<TooltipContent>Open in chat</TooltipContent>
								</Tooltip>
								<Button
									variant="subtle"
									size="icon"
									onClick={() => onOpenChange(false)}
									aria-label="Close"
								>
									<XIcon className="size-4" />
								</Button>
							</div>
						</div>

						{/* Review Banner */}
						<ReviewBanner chat={currentChat} />

						{/* Chat Content */}
						<div className="flex-1 min-h-0">
							<ReviewChatContent chatId={currentChat.id} />
						</div>

						{/* Reply Input */}
						<ReviewReplyInput
							onSend={(message) => {
								onOpenChange(false);
								navigate(`/agents/${currentChat.id}`, {
									state: { pendingMessage: message },
								});
							}}
						/>

						{/* Navigation Bar */}
						<ReviewNavigationBar
							currentIndex={currentIndex}
							totalCount={snapshotChats.length}
							onPrevious={handlePrevious}
							onNext={handleNext}
							isLast={currentIndex >= snapshotChats.length - 1}
						/>
					</>
				) : (
					<>
						<div className="flex items-center justify-between border-b border-border-default px-6 py-4">
							<DialogTitle className="text-lg">Review unread chats</DialogTitle>
							<Button
								variant="subtle"
								size="icon"
								onClick={() => onOpenChange(false)}
								aria-label="Close"
							>
								<XIcon className="size-4" />
							</Button>
						</div>
						<div className="flex flex-1 items-center justify-center">
							<p className="text-sm text-content-secondary">
								No agent chats to review
							</p>
						</div>
					</>
				)}
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
	const bottomRef = useRef<HTMLDivElement>(null);
	const messagesQuery = useInfiniteQuery({
		...chatMessagesForInfiniteScroll(chatId),
	});

	const allMessages: ChatMessage[] =
		messagesQuery.data?.pages.flatMap((page) => page.messages).reverse() ?? [];

	const parsedMessages = parseMessagesWithMergedTools(allMessages);
	const subagentTitles = buildSubagentTitles(parsedMessages);
	const computerUseSubagentIds = buildComputerUseSubagentIds(parsedMessages);

	// Scroll to the bottom once messages finish loading so the
	// user sees the most recent activity first. A double-rAF
	// ensures the browser has completed layout after React
	// commits the DOM update.
	useEffect(() => {
		if (!messagesQuery.isLoading && allMessages.length > 0) {
			requestAnimationFrame(() => {
				requestAnimationFrame(() => {
					bottomRef.current?.scrollIntoView({ block: "end" });
				});
			});
		}
	}, [messagesQuery.isLoading, allMessages.length]);

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
		<div className="h-full overflow-y-auto">
			<div className="mx-auto flex w-full max-w-3xl flex-col gap-2 py-6">
				<ConversationTimeline
					parsedMessages={parsedMessages}
					subagentTitles={subagentTitles}
					computerUseSubagentIds={computerUseSubagentIds}
					showDesktopPreviews={false}
				/>
				<div ref={bottomRef} />
			</div>
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

interface ReviewReplyInputProps {
	onSend: (message: string) => void;
}

const ReviewReplyInput: FC<ReviewReplyInputProps> = ({ onSend }) => {
	const [value, setValue] = useState("");
	const textareaRef = useRef<HTMLTextAreaElement>(null);

	const handleSubmit = () => {
		const trimmed = value.trim();
		if (!trimmed) return;
		onSend(trimmed);
		setValue("");
	};

	const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSubmit();
		}
	};

	// Auto-resize the textarea to fit content.
	// biome-ignore lint/correctness/useExhaustiveDependencies: value drives the resize
	useEffect(() => {
		const el = textareaRef.current;
		if (!el) return;
		el.style.height = "auto";
		el.style.height = `${Math.min(el.scrollHeight, 120)}px`;
	}, [value]);

	return (
		<div className="border-t border-border-default px-6 py-3">
			<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm has-[textarea:focus]:ring-2 has-[textarea:focus]:ring-content-link/40">
				<textarea
					ref={textareaRef}
					value={value}
					onChange={(e) => setValue(e.target.value)}
					onKeyDown={handleKeyDown}
					placeholder="Reply to this agent…"
					rows={1}
					className="min-h-[44px] w-full resize-none bg-transparent px-3 py-2 font-sans text-[15px] leading-6 text-content-primary placeholder:text-content-secondary focus:outline-none"
				/>
				<div className="flex items-center justify-end px-2.5 pb-1.5">
					<Button
						size="icon"
						variant="default"
						className="size-7 rounded-full [&>svg]:!size-5 [&>svg]:p-0"
						onClick={handleSubmit}
						disabled={!value.trim()}
						aria-label="Send and open chat"
					>
						<ArrowUpIcon />
					</Button>
				</div>
			</div>
		</div>
	);
};
