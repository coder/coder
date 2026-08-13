import {
	MessageScroller,
	useMessageScroller,
} from "@shadcn/react/message-scroller";
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	InfoIcon,
	PencilIcon,
} from "lucide-react";
import { type FC, memo, type ReactNode, useState } from "react";

import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";

import { AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

import {
	ConversationItem,
	Message,
	MessageContent,
	Response,
} from "../ChatElements";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import { AssistantOutput } from "./AssistantOutput";
import type { PreviewTextAttachment } from "./AttachmentBlocks";
import { FileProbeProvider } from "./FileProbeContext";
import {
	type LiveStatusModel,
	shouldRenderLiveAssistant,
} from "./liveStatusModel";
import {
	buildDisplayMessages,
	deriveMessageDisplayState,
} from "./messageHelpers";
import { getEditableUserMessagePayload } from "./messageParsing";
import { assignTimelineRows } from "./timelineRows";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";
import { UserMessageContent } from "./UserMessageContent";

const getChatMessageTextContent = (
	content: readonly TypesGen.ChatMessagePart[] | undefined,
): string | undefined => {
	if (!content) {
		return undefined;
	}

	let textContent = "";
	for (const part of content) {
		if (part.type === "text") {
			textContent += part.text;
		}
	}

	return textContent.length > 0 ? textContent : undefined;
};

// Avoid announcing historical hook notices as live alerts.
const TimelineNotice: FC<{ children?: ReactNode }> = ({ children }) => (
	<div
		role="note"
		className="relative my-1 w-full rounded-lg border border-solid border-border-default bg-surface-secondary p-4 text-left"
	>
		<div className="flex min-w-0 flex-1 flex-row items-start gap-3 text-sm">
			<InfoIcon className="size-icon-sm mt-[3px] text-highlight-sky" />
			<div className="min-w-0 flex-1">{children}</div>
		</div>
	</div>
);

const LifecycleHookNotice: FC<{
	children: string;
	urlTransform?: UrlTransform;
}> = ({ children, urlTransform }) => (
	<TimelineNotice>
		<div className="flex flex-col gap-1">
			<AlertTitle>Lifecycle hook</AlertTitle>
			<Response urlTransform={urlTransform}>{children}</Response>
		</div>
	</TimelineNotice>
);

const ChatMessageItem = memo<{
	renderKey: string;
	// Durable messages and live assistant output share one rendering path.
	message?: TypesGen.ChatMessage;
	parsed?: ParsedMessageContent;
	liveStatus?: LiveStatusModel;
	// Live blocks and tools are normalized at the live row callsite, so this
	// component never has to decide when stream output is visible.
	liveBlocks?: readonly RenderBlock[];
	liveTools?: readonly MergedTool[];
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	isAfterEditingMessage?: boolean;
	hideActions?: boolean;
	hasActiveStream?: boolean;
	isAwaitingFirstStreamChunk?: boolean;

	// The bottom spacer fakes the height of the hidden action bar so
	// chain-end messages keep even spacing before the next bubble.
	// The last transcript message has nothing after it, so the spacer
	// would render as a dangling blank at the end of the chat.
	isLastMessage?: boolean;
	onImplementPlan?: () => Promise<void> | void;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	showDesktopPreviews?: boolean;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	latestAskUserQuestionToolId?: string;
	askUserQuestionResponseTextByToolId?: ReadonlyMap<string, string>;
	hasUserResponseAfterAskQuestion?: boolean;
	prevUserMessageKey?: string;
	nextUserMessageKey?: string;
	onJumpToUserMessage?: (messageKey: string) => void;
}>(
	({
		renderKey,
		message,
		parsed,
		liveStatus,
		liveBlocks = [],
		liveTools = [],
		subagentStatusOverrides,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
		hideActions = false,
		hasActiveStream = false,
		isAwaitingFirstStreamChunk = false,
		isLastMessage = false,
		onImplementPlan,
		onSendAskUserQuestionResponse,
		isChatCompleted,
		latestAskUserQuestionToolId,
		askUserQuestionResponseTextByToolId,
		hasUserResponseAfterAskQuestion = false,
		prevUserMessageKey,
		nextUserMessageKey,
		onJumpToUserMessage,

		urlTransform,
		mcpServers,
		subagentTitles,
		subagentVariants,
		showDesktopPreviews,
	}) => {
		const isUser = message?.role === "user";
		const messageId = message?.id;
		const [previewImage, setPreviewImage] = useState<string | null>(null);
		const [previewText, setPreviewText] =
			useState<PreviewTextAttachment | null>(null);
		const displayState =
			message && parsed
				? deriveMessageDisplayState({
						message,
						parsed,
						hideActions,
						hasActiveStream,
						isAwaitingFirstStreamChunk,
					})
				: undefined;
		if (displayState?.shouldHide) {
			return null;
		}
		if (message?.role === "system" && parsed) {
			return (
				<div
					className={cn(
						isAfterEditingMessage && "opacity-40 pointer-events-none",
						"transition-opacity duration-200",
					)}
					// Keep links in dimmed notices out of accessibility navigation.
					inert={isAfterEditingMessage ? true : undefined}
				>
					{parsed.hookNotices.length > 0 ? (
						parsed.hookNotices.map((notice, index) => (
							<LifecycleHookNotice
								key={`${renderKey}-hook-notice-${index}`}
								urlTransform={urlTransform}
							>
								{notice}
							</LifecycleHookNotice>
						))
					) : (
						<TimelineNotice>
							<Response urlTransform={urlTransform}>{parsed.markdown}</Response>
						</TimelineNotice>
					)}
				</div>
			);
		}

		const conversationItemProps: { role: "user" | "assistant" } = {
			role: isUser ? "user" : "assistant",
		};

		return (
			<div
				data-testid={`chat-message-${renderKey}`}
				className={cn(
					isAfterEditingMessage && "opacity-40 pointer-events-none",
					"group/msg relative transition-opacity duration-200",
				)}
				inert={isAfterEditingMessage ? true : undefined}
			>
				<ConversationItem {...conversationItemProps}>
					{isUser && displayState && parsed ? (
						<UserMessageContent
							displayState={displayState}
							markdown={parsed.markdown}
							isEditing={
								messageId !== undefined && editingMessageId === messageId
							}
							onImageClick={setPreviewImage}
							onTextFileClick={setPreviewText}
						/>
					) : (
						<Message className="w-full">
							<MessageContent className="whitespace-normal">
								<AssistantOutput
									keyPrefix={renderKey}
									blocks={parsed?.blocks ?? liveBlocks}
									tools={parsed?.tools ?? liveTools}
									isStreaming={liveStatus?.phase === "streaming"}
									liveStatus={liveStatus}
									subagentStatusOverrides={subagentStatusOverrides}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									showDesktopPreviews={showDesktopPreviews}
									onImplementPlan={onImplementPlan}
									onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
									isChatCompleted={isChatCompleted}
									latestAskUserQuestionToolId={latestAskUserQuestionToolId}
									askUserQuestionResponseTextByToolId={
										askUserQuestionResponseTextByToolId
									}
									hasUserResponseAfterAskQuestion={
										hasUserResponseAfterAskQuestion
									}
									onImageClick={setPreviewImage}
									onTextFileClick={setPreviewText}
									urlTransform={urlTransform}
									mcpServers={mcpServers}
								/>
							</MessageContent>
						</Message>
					)}
				</ConversationItem>
				{parsed?.hookNotices.map((notice, index) => (
					<LifecycleHookNotice
						key={`${renderKey}-hook-notice-${index}`}
						urlTransform={urlTransform}
					>
						{notice}
					</LifecycleHookNotice>
				))}
				{displayState &&
					!hideActions &&
					(displayState.hasCopyableContent ||
						(isUser && onEditUserMessage)) && (
						<div
							className={cn(
								"mt-0.5 flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100",
								isUser && "w-full justify-end",
							)}
							data-testid="message-actions"
						>
							{displayState.hasCopyableContent && parsed && (
								<CopyButton
									text={parsed.markdown}
									label="Copy message"
									className="size-6"
									tooltipSide="bottom"
								/>
							)}
							{isUser && messageId !== undefined && onEditUserMessage && (
								<Tooltip>
									<TooltipTrigger asChild>
										<Button
											size="icon"
											variant="subtle"
											className="size-6"
											aria-label="Edit message"
											onClick={() => {
												const { text, fileBlocks } =
													getEditableUserMessagePayload(message);
												onEditUserMessage(messageId, text, fileBlocks);
											}}
										>
											<PencilIcon />
											<span className="sr-only">Edit message</span>
										</Button>
									</TooltipTrigger>
									<TooltipContent side="bottom">Edit message</TooltipContent>
								</Tooltip>
							)}
							{isUser &&
								onJumpToUserMessage &&
								(prevUserMessageKey !== undefined ||
									nextUserMessageKey !== undefined) && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													className="size-6"
													aria-label="Jump to previous user message"
													disabled={prevUserMessageKey === undefined}
													onClick={() => {
														if (prevUserMessageKey !== undefined) {
															onJumpToUserMessage(prevUserMessageKey);
														}
													}}
												>
													<ChevronLeftIcon />
													<span className="sr-only">
														Jump to previous user message
													</span>
												</Button>
											</TooltipTrigger>
											<TooltipContent side="bottom">
												Jump to previous user message
											</TooltipContent>
										</Tooltip>
										<Tooltip>
											<TooltipTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													className="size-6"
													aria-label="Jump to next user message"
													disabled={nextUserMessageKey === undefined}
													onClick={() => {
														if (nextUserMessageKey !== undefined) {
															onJumpToUserMessage(nextUserMessageKey);
														}
													}}
												>
													<ChevronRightIcon />
													<span className="sr-only">
														Jump to next user message
													</span>
												</Button>
											</TooltipTrigger>
											<TooltipContent side="bottom">
												Jump to next user message
											</TooltipContent>
										</Tooltip>
									</>
								)}
						</div>
					)}
				{displayState?.needsAssistantBottomSpacer && !isLastMessage && (
					<div className="min-h-6" data-testid="assistant-bottom-spacer" />
				)}
				{previewImage && (
					<ImageLightbox
						src={previewImage}
						onClose={() => setPreviewImage(null)}
					/>
				)}
				{previewText !== null && (
					<TextPreviewDialog
						content={previewText.content}
						fileName={previewText.fileName}
						mediaType={previewText.mediaType}
						onClose={() => setPreviewText(null)}
					/>
				)}
			</div>
		);
	},
);

interface ConversationTimelineProps {
	parsedMessages: readonly ParsedMessageEntry[];
	onVisibleRowsChange?: (hasVisibleRows: boolean) => void;
	streamState?: StreamState | null;
	streamTools?: readonly MergedTool[];
	liveStatus?: LiveStatusModel;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	subagentTitles: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	urlTransform?: UrlTransform;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	showDesktopPreviews?: boolean;
	hasActiveStream?: boolean;
	isAwaitingFirstStreamChunk?: boolean;
}

export const ConversationTimeline = memo<ConversationTimelineProps>(
	({
		parsedMessages,
		onVisibleRowsChange,
		streamState,
		streamTools = [],
		liveStatus,
		subagentStatusOverrides,
		subagentTitles,
		subagentVariants,
		onEditUserMessage,
		editingMessageId,
		onImplementPlan,
		onSendAskUserQuestionResponse,
		isChatCompleted,
		urlTransform,
		mcpServers,
		showDesktopPreviews,
		hasActiveStream,
		isAwaitingFirstStreamChunk,
	}) => {
		const { scrollToMessage } = useMessageScroller();
		const jumpToUserMessage = (messageKey: string) => {
			scrollToMessage(messageKey, { align: "start", behavior: "smooth" });
		};

		const displayMessages = buildDisplayMessages(parsedMessages);
		const renderRows = assignTimelineRows(
			displayMessages,
			Boolean(liveStatus && shouldRenderLiveAssistant(liveStatus)),
		);
		const hasVisibleRows = displayMessages.length > 0;
		const [reportedVisibleRows, setReportedVisibleRows] = useState<boolean>();
		if (hasVisibleRows !== reportedVisibleRows) {
			setReportedVisibleRows(hasVisibleRows);
			onVisibleRowsChange?.(hasVisibleRows);
		}

		// A live turn only reveals its stream blocks once output has accumulated.
		// Before that the callout and thinking indicator stand in for the turn.
		const showsStreamOutput =
			liveStatus !== undefined &&
			(liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput);
		const liveBlocks = showsStreamOutput ? (streamState?.blocks ?? []) : [];
		const liveTools = showsStreamOutput ? streamTools : [];

		if (renderRows.length === 0) {
			return null;
		}

		// Build a set of message IDs that appear after the message
		// currently being edited so they can be visually faded.
		const afterEditingMessageIds = new Set<number>();
		if (editingMessageId != null) {
			let found = false;
			for (const entry of parsedMessages) {
				if (entry.message.id === editingMessageId) {
					found = true;
					continue;
				}
				if (found) {
					afterEditingMessageIds.add(entry.message.id);
				}
			}
		}

		// Ordered list of visible user rows, used to drive the per-bubble
		// prev/next arrow buttons that jump the transcript to the neighbouring
		// user prompt. The row key doubles as the scroller's message ID.
		const userRowKeys: string[] = [];
		for (const row of renderRows) {
			if (row.type === "message" && row.entry.message.role === "user") {
				userRowKeys.push(row.key);
			}
		}
		// Only the latest user row anchors the scroller, and only while its turn
		// is active. The scroller never marks initially rendered anchors as
		// handled, and its fallback for mutations that are neither clean appends
		// nor clean prepends jumps to the oldest unhandled anchor, so historical
		// rows must not be anchors at all.
		const hasLiveAssistant = Boolean(
			liveStatus && shouldRenderLiveAssistant(liveStatus),
		);
		const anchorUserRowKey =
			hasLiveAssistant || isAwaitingFirstStreamChunk
				? userRowKeys[userRowKeys.length - 1]
				: undefined;
		const userNeighborsByKey = new Map<
			string,
			{ prevKey?: string; nextKey?: string }
		>();
		for (let i = 0; i < userRowKeys.length; i++) {
			userNeighborsByKey.set(userRowKeys[i], {
				prevKey: i > 0 ? userRowKeys[i - 1] : undefined,
				nextKey: i < userRowKeys.length - 1 ? userRowKeys[i + 1] : undefined,
			});
		}
		let latestAskUserQuestionToolId: string | undefined;
		let hasUserResponseAfterAskQuestion = false;
		const askUserQuestionResponseTextByToolId = new Map<string, string>();
		let pendingAskUserQuestionToolId: string | undefined;
		for (const { message, parsed } of parsedMessages) {
			let askUserQuestionToolIdInMessage: string | undefined;
			for (const tool of parsed.tools) {
				if (tool.name === "ask_user_question") {
					askUserQuestionToolIdInMessage = tool.id;
					latestAskUserQuestionToolId = tool.id;
					hasUserResponseAfterAskQuestion = false;
				}
			}

			if (askUserQuestionToolIdInMessage) {
				pendingAskUserQuestionToolId = askUserQuestionToolIdInMessage;
			}

			if (pendingAskUserQuestionToolId && message.role === "user") {
				hasUserResponseAfterAskQuestion =
					pendingAskUserQuestionToolId === latestAskUserQuestionToolId;
				const responseText = getChatMessageTextContent(message.content);
				if (responseText !== undefined) {
					askUserQuestionResponseTextByToolId.set(
						pendingAskUserQuestionToolId,
						responseText,
					);
				}
				pendingAskUserQuestionToolId = undefined;
			}
		}
		const historicalAskUserQuestionResponseTextByToolId =
			askUserQuestionResponseTextByToolId.size > 0
				? askUserQuestionResponseTextByToolId
				: undefined;

		return (
			<FileProbeProvider>
				{renderRows.map((row) => {
					if (row.type === "live") {
						// This row only exists when liveStatus is set.
						return (
							<MessageScroller.Item key={row.key} messageId={row.key}>
								<ChatMessageItem
									renderKey={row.key}
									liveStatus={liveStatus}
									liveBlocks={liveBlocks}
									liveTools={liveTools}
									subagentStatusOverrides={
										showsStreamOutput ? subagentStatusOverrides : undefined
									}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									urlTransform={urlTransform}
									mcpServers={mcpServers}
								/>
							</MessageScroller.Item>
						);
					}
					const { message, parsed } = row.entry;
					const isUser = message.role === "user";
					const neighbors = userNeighborsByKey.get(row.key);
					const isAfterEditingMessage = afterEditingMessageIds.has(message.id);
					return (
						<MessageScroller.Item
							key={row.key}
							messageId={row.key}
							scrollAnchor={isUser && row.key === anchorUserRowKey}
						>
							<ChatMessageItem
								renderKey={row.key}
								message={message}
								parsed={parsed}
								onEditUserMessage={isUser ? onEditUserMessage : undefined}
								editingMessageId={editingMessageId}
								onImplementPlan={onImplementPlan}
								onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
								isChatCompleted={isChatCompleted}
								latestAskUserQuestionToolId={latestAskUserQuestionToolId}
								askUserQuestionResponseTextByToolId={
									historicalAskUserQuestionResponseTextByToolId
								}
								hasUserResponseAfterAskQuestion={
									hasUserResponseAfterAskQuestion
								}
								urlTransform={urlTransform}
								isAfterEditingMessage={isAfterEditingMessage}
								hideActions={!isUser && !row.isLastInAssistantChain}
								hasActiveStream={Boolean(hasActiveStream)}
								isAwaitingFirstStreamChunk={Boolean(isAwaitingFirstStreamChunk)}
								isLastMessage={row.isLastMessage}
								mcpServers={mcpServers}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								showDesktopPreviews={showDesktopPreviews}
								prevUserMessageKey={neighbors?.prevKey}
								nextUserMessageKey={neighbors?.nextKey}
								onJumpToUserMessage={isUser ? jumpToUserMessage : undefined}
							/>
						</MessageScroller.Item>
					);
				})}
			</FileProbeProvider>
		);
	},
);
