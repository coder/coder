import { cn } from "cn";
import {
	ChevronLeftIcon,
	ChevronRightIcon,
	InfoIcon,
	PencilIcon,
} from "lucide-react";
import { memo, useState } from "react";
import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { MessageScroller, useMessageScroller } from "#/vendor/message-scroller";
import { ConversationItem } from "../ChatElements/Conversation";
import { Message, MessageContent } from "../ChatElements/Message";
import { Response } from "../ChatElements/Response";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import { AssistantOutput } from "./AssistantOutput";
import type { PreviewTextAttachment } from "./AttachmentBlocks";
import { FileProbeProvider } from "./FileProbeContext";
import {
	emptyLiveBlockKeys,
	reconcileLiveBlockKeys,
} from "./liveBlockItemKeys";
import {
	type LiveStatusModel,
	shouldRenderLiveAssistant,
} from "./liveStatusModel";
import {
	buildDisplayMessages,
	deriveEvictedFileIds,
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
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";
import {
	groupWorkingBlocks,
	type RowSlice,
	splitRowPieces,
	type TimelineItem,
} from "./workingBlockGrouping";

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
const TimelineNotice: React.FC<{ children?: React.ReactNode }> = ({
	children,
}) => (
	<div
		role="note"
		className="relative my-1 w-full rounded-lg border border-solid border-border bg-surface-secondary p-4 text-left"
	>
		<div className="flex min-w-0 flex-1 flex-row items-start gap-3 text-sm">
			<InfoIcon className="size-icon-sm mt-[3px] text-highlight-sky" />
			<div className="min-w-0 flex-1">{children}</div>
		</div>
	</div>
);

const LifecycleHookNotice: React.FC<{
	children: string;
	urlTransform: UrlTransform;
}> = ({ children, urlTransform }) => (
	<TimelineNotice>
		<div className="flex flex-col gap-1">
			<AlertTitle>Lifecycle hook</AlertTitle>
			<Response urlTransform={urlTransform}>{children}</Response>
		</div>
	</TimelineNotice>
);

const ChatMessageItem = memo<{
	organizationId: string;
	renderKey: string;
	// Durable messages and live assistant output share one rendering path.
	message?: TypesGen.ChatMessage;
	// A row that a working block splits renders as slices, each a half-open
	// range of its pieces from splitRowPieces.
	pieceStart?: number;
	pieceEnd?: number;
	isWorkSlice?: boolean;
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
	urlTransform: UrlTransform;
	mcpServers: readonly TypesGen.MCPServerConfig[];
	subagentTitles: Map<string, string>;
	subagentVariants: Map<string, SubagentVariant>;
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
		organizationId,
		renderKey,
		message,
		pieceStart,
		pieceEnd,
		isWorkSlice = false,
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
		const rowBlocks = parsed?.blocks ?? liveBlocks;
		const rowTools = parsed?.tools ?? liveTools;
		const pieces =
			pieceStart === undefined
				? undefined
				: splitRowPieces(rowBlocks, rowTools);
		const blocks = pieces
			? pieces.slice(pieceStart, pieceEnd).flatMap((piece) => piece.blocks)
			: rowBlocks;
		// A slice renders only its own tools, or the rest would render again
		// as block-less tools.
		const sliceToolIds = new Set(
			blocks.flatMap((block) => (block.type === "tool" ? [block.id] : [])),
		);
		const tools = pieces
			? rowTools.filter((tool) => sliceToolIds.has(tool.id))
			: rowTools;
		// The slice that ends the row carries its actions and live status.
		const endsRow = !pieces || pieceEnd === pieces.length;
		const coversRow = endsRow && (pieceStart ?? 0) === 0;
		const showsActions = !hideActions && endsRow;
		const displayState =
			message && parsed
				? deriveMessageDisplayState({
						message,
						parsed,
						hideActions: !showsActions,
						hasActiveStream,
						isAwaitingFirstStreamChunk,
					})
				: undefined;
		const canEditUserMessage =
			isUser && messageId !== undefined && Boolean(onEditUserMessage);
		const canJumpBetweenUserMessages =
			isUser &&
			Boolean(onJumpToUserMessage) &&
			(prevUserMessageKey !== undefined || nextUserMessageKey !== undefined);
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
		const outputLiveStatus = endsRow ? liveStatus : undefined;

		return (
			<div
				data-testid={`chat-message-${renderKey}${isWorkSlice && !coversRow ? "-work" : ""}`}
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
									organizationId={organizationId}
									keyPrefix={renderKey}
									blocks={blocks}
									tools={tools}
									isStreaming={outputLiveStatus?.phase === "streaming"}
									liveStatus={outputLiveStatus}
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
					showsActions &&
					(displayState.hasCopyableContent ||
						canEditUserMessage ||
						canJumpBetweenUserMessages) && (
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
							{canEditUserMessage && messageId !== undefined && (
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
												onEditUserMessage?.(messageId, text, fileBlocks);
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

type ConversationTimelineProps = {
	hasMoreMessages: boolean;
	chatStatus: TypesGen.ChatStatus | null;
	organizationId: string;
	parsedMessages: readonly ParsedMessageEntry[];
	chatFiles?: readonly TypesGen.ChatFileMetadata[];
	initialActiveTurnMaxMessageId?: number;
	streamState?: StreamState | null;
	streamTools: readonly MergedTool[];
	liveStatus: LiveStatusModel;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	subagentTitles: Map<string, string>;
	subagentVariants: Map<string, SubagentVariant>;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted: boolean;
	urlTransform: UrlTransform;
	mcpServers: readonly TypesGen.MCPServerConfig[];
	showDesktopPreviews: boolean;
	hasActiveStream: boolean;
	isAwaitingFirstStreamChunk: boolean;
};

export const ConversationTimeline = memo<ConversationTimelineProps>(
	({
		hasMoreMessages,
		chatStatus,
		organizationId,
		parsedMessages,
		chatFiles,
		initialActiveTurnMaxMessageId,
		streamState,
		streamTools,
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
		const preferences = useQuery(preferenceSettings());
		const [expandedBlocks, setExpandedBlocks] = useState<
			ReadonlyMap<string, boolean>
		>(new Map());
		const [liveBlockKeys, setLiveBlockKeys] = useState(emptyLiveBlockKeys);

		const jumpToUserMessage = (messageKey: string) => {
			scrollToMessage(messageKey, { align: "start", behavior: "smooth" });
		};

		const displayMessages = buildDisplayMessages(parsedMessages);
		const evictedFileIds = deriveEvictedFileIds(parsedMessages, chatFiles);
		const renderRows = assignTimelineRows(
			displayMessages,
			shouldRenderLiveAssistant(liveStatus),
		);

		// A live turn only reveals its stream blocks once output has accumulated.
		// Before that the callout and thinking indicator stand in for the turn.
		const showsStreamOutput =
			liveStatus.phase === "streaming" || liveStatus.hasAccumulatedOutput;
		const liveBlocks = showsStreamOutput ? (streamState?.blocks ?? []) : [];
		const liveTools = showsStreamOutput ? streamTools : [];

		const layout = preferences.data?.collapse_assistant_steps
			? groupWorkingBlocks(renderRows, parsedMessages, {
					hasMoreMessages,
					// requires_action is the agent waiting on the user, not working.
					isTurnActive:
						chatStatus === "running" || chatStatus === "interrupting",
					isWorking: chatStatus === "running",
					isLiveRowCollapsible:
						liveStatus.phase === "streaming" || liveStatus.phase === "starting",
					liveBlocks,
					liveTools,
					streamState,
				})
			: undefined;
		const workingBlocks = layout?.blocks ?? [];
		const items =
			layout?.items ??
			renderRows.map(
				(row, rowIndex): TimelineItem => ({
					type: "slice",
					slice: { rowIndex, key: row.key },
				}),
			);
		const nextLiveBlockKeys = reconcileLiveBlockKeys(
			workingBlocks,
			streamState?.startedAt,
			liveBlockKeys,
		);
		if (nextLiveBlockKeys !== liveBlockKeys) {
			setLiveBlockKeys(nextLiveBlockKeys);
		}

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
		const hasLiveAssistant = shouldRenderLiveAssistant(liveStatus);
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

		// A slice keeps the row's key while it holds the row's first work or
		// output, so a row that splits inside an expanded block does not remount
		// its reasoning or streaming text.
		const renderSlice = (slice: RowSlice, inBlock: boolean) => {
			const row = renderRows[slice.rowIndex];
			const sliceProps = {
				renderKey: slice.key,
				pieceStart: slice.pieces?.start,
				pieceEnd: slice.pieces?.end,
				isWorkSlice: inBlock,
			};
			if (row.type === "live") {
				return (
					<ChatMessageItem
						key={slice.key}
						{...sliceProps}
						organizationId={organizationId}
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
				);
			}

			const { message, parsed } = row.entry;
			const isUser = message.role === "user";
			const neighbors = userNeighborsByKey.get(row.key);

			return (
				<ChatMessageItem
					key={slice.key}
					{...sliceProps}
					organizationId={organizationId}
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
					hasUserResponseAfterAskQuestion={hasUserResponseAfterAskQuestion}
					urlTransform={urlTransform}
					// A block's item dims and inerts its slices as a whole.
					isAfterEditingMessage={
						!inBlock && afterEditingMessageIds.has(message.id)
					}
					hideActions={inBlock || (!isUser && !row.isLastInAssistantChain)}
					hasActiveStream={hasActiveStream}
					isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
					isLastMessage={row.isLastMessage}
					mcpServers={mcpServers}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					showDesktopPreviews={showDesktopPreviews}
					prevUserMessageKey={neighbors?.prevKey}
					nextUserMessageKey={neighbors?.nextKey}
					onJumpToUserMessage={isUser ? jumpToUserMessage : undefined}
				/>
			);
		};

		return (
			<FileProbeProvider evictedFileIds={evictedFileIds}>
				{items.map((item) => {
					if (item.type === "block") {
						const { block } = item;
						// Keyed on durable pieces and the item key, excluding the live
						// row so the choice does not carry into the next turn; the newest
						// piece wins.
						const itemKey =
							nextLiveBlockKeys.itemKeys.get(block.key) ?? block.key;
						const expanded =
							expandedBlocks.get(
								block.pieceKeys.findLast((key) => expandedBlocks.has(key)) ??
									itemKey,
							) ?? false;
						const firstRow = renderRows[block.slices[0].rowIndex];
						const isAfterEditingMessage =
							firstRow.type === "message" &&
							afterEditingMessageIds.has(firstRow.entry.message.id);

						return (
							<MessageScroller.Item
								key={itemKey}
								messageId={itemKey}
								className={cn(
									isAfterEditingMessage && "opacity-40 pointer-events-none",
									"transition-opacity duration-200",
								)}
								inert={isAfterEditingMessage ? true : undefined}
							>
								<WorkingBlockDisclosure
									block={block}
									expanded={expanded}
									onExpandedChange={(value) =>
										setExpandedBlocks((previous) => {
											const next = new Map(previous);
											for (const key of [itemKey, ...block.pieceKeys]) {
												next.set(key, value);
											}
											return next;
										})
									}
								>
									{expanded &&
										block.slices.map((slice) => renderSlice(slice, true))}
								</WorkingBlockDisclosure>
							</MessageScroller.Item>
						);
					}

					const { slice } = item;
					const row = renderRows[slice.rowIndex];
					const suppressInitialAnchor =
						row.type === "message" &&
						initialActiveTurnMaxMessageId !== undefined &&
						row.entry.message.id <= initialActiveTurnMaxMessageId;
					return (
						<MessageScroller.Item
							key={slice.key}
							messageId={slice.key}
							scrollAnchor={
								slice.key === anchorUserRowKey && !suppressInitialAnchor
							}
						>
							{renderSlice(slice, false)}
						</MessageScroller.Item>
					);
				})}
			</FileProbeProvider>
		);
	},
);
