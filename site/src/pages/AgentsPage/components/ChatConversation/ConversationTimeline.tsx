import { PencilIcon } from "lucide-react";
import { type FC, Fragment, memo, useEffect, useRef, useState } from "react";
import type { UrlTransform } from "streamdown";
import type * as TypesGen from "#/api/typesGenerated";
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
	Shimmer,
	Tool,
} from "../ChatElements";
import { WebSearchSources } from "../ChatElements/tools";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import {
	getPinnedPreviewMetrics,
	PINNED_PREVIEW_MIN_HEIGHT_PX,
} from "../chatViewportUtils";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import { ExpiredFileIdsProvider } from "./ExpiredFileIdsContext";
import { deriveMessageDisplayState } from "./messageHelpers";
import { getEditableUserMessagePayload } from "./messageParsing";
import { useSmoothStreamingText } from "./SmoothText";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	RenderBlock,
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

const ReasoningDisclosure = memo<{
	id: string;
	text: string;
	isStreaming?: boolean;
	urlTransform?: UrlTransform;
}>(({ id, text, isStreaming = false, urlTransform }) => {
	const { visibleText } = useSmoothStreamingText({
		fullText: text,
		isStreaming,
		bypassSmoothing: !isStreaming,
		streamKey: id,
	});
	const displayText = isStreaming ? visibleText : text;
	const hasText = displayText.trim().length > 0;

	if (hasText) {
		return (
			<div className="w-full">
				<Response
					className="text-[11px] text-content-secondary"
					urlTransform={urlTransform}
					streaming={isStreaming}
				>
					{displayText}
				</Response>
			</div>
		);
	}

	return (
		<div className="w-full">
			<div className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary">
				<span className="text-sm">
					{isStreaming ? <Shimmer as="span">Thinking...</Shimmer> : "Thinking"}
				</span>
			</div>
		</div>
	);
});

// Wrapper that runs the smooth-streaming jitter buffer on a single
// response block. Only used during live streaming — historical
// messages render through <Response> directly.
const SmoothedResponse = memo<{
	text: string;
	streamKey: string;
	urlTransform?: UrlTransform;
}>(({ text, streamKey, urlTransform }) => {
	const { visibleText } = useSmoothStreamingText({
		fullText: text,
		isStreaming: true,
		bypassSmoothing: false,
		streamKey,
	});
	return (
		<Response streaming urlTransform={urlTransform}>
			{visibleText}
		</Response>
	);
});

// Shared block renderer used by both ChatMessageItem (historical
// messages) and StreamingOutput (live stream). Encapsulates the
// response / thinking / tool / file / sources switch so both
// consumers stay in sync. PascalCase so the React Compiler
// auto-memoizes every element inside.
export const BlockList: FC<{
	blocks: readonly RenderBlock[];
	tools: readonly MergedTool[];
	keyPrefix: string;
	isStreaming?: boolean;
	subagentTitles?: Map<string, string>;
	subagentVariants?: Map<string, SubagentVariant>;
	showDesktopPreviews?: boolean;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	mcpServers?: readonly TypesGen.MCPServerConfig[];
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
	onImplementPlan?: () => Promise<void> | void;
	onSendAskUserQuestionResponse?: (message: string) => Promise<void> | void;
	isChatCompleted?: boolean;
	latestAskUserQuestionToolId?: string;
	askUserQuestionResponseTextByToolId?: ReadonlyMap<string, string>;
	hasUserResponseAfterAskQuestion?: boolean;
	urlTransform?: UrlTransform;
}> = ({
	blocks,
	tools,
	keyPrefix,
	isStreaming = false,
	subagentTitles,
	subagentVariants,
	showDesktopPreviews,
	subagentStatusOverrides,
	mcpServers,
	onImageClick,
	onTextFileClick,
	onImplementPlan,
	onSendAskUserQuestionResponse,
	isChatCompleted,
	latestAskUserQuestionToolId,
	askUserQuestionResponseTextByToolId,
	hasUserResponseAfterAskQuestion = false,
	urlTransform,
}) => {
	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));

	// Pre-compute which tool IDs have a corresponding block so
	// we can render "remaining" (block-less) tools afterwards.
	const blockToolIDs = new Set(
		blocks
			.filter(
				(b): b is Extract<RenderBlock, { type: "tool" }> =>
					b.type === "tool" && (toolByID.has(b.id) || isStreaming),
			)
			.map((b) => b.id),
	);

	const remainingTools = tools.filter((tool) => !blockToolIDs.has(tool.id));

	return (
		<>
			{blocks.map((block, index) => {
				switch (block.type) {
					case "response": {
						const responseEl = isStreaming ? (
							<SmoothedResponse
								key={`${keyPrefix}-response-${index}`}
								text={block.text}
								streamKey={keyPrefix}
								urlTransform={urlTransform}
							/>
						) : (
							<Response
								key={`${keyPrefix}-response-${index}`}
								urlTransform={urlTransform}
							>
								{block.text}
							</Response>
						);
						return (
							<Fragment key={`${keyPrefix}-response-${index}`}>
								{responseEl}
							</Fragment>
						);
					}
					case "thinking":
						return (
							<ReasoningDisclosure
								key={`${keyPrefix}-thinking-${index}`}
								id={`${keyPrefix}-thinking-${index}`}
								text={block.text}
								isStreaming={isStreaming}
								urlTransform={urlTransform}
							/>
						);
					case "file-reference":
						return (
							<div
								key={`${keyPrefix}-file-reference-${index}`}
								className="my-1 flex items-start gap-2 rounded-md border border-content-link/20 bg-content-link/5 px-2.5 py-1.5"
							>
								<span className="shrink-0 text-xs font-medium text-content-link">
									{block.file_name}:
									{block.start_line === block.end_line
										? block.start_line
										: `${block.start_line}\u2013${block.end_line}`}
								</span>
							</div>
						);
					case "tool": {
						const tool = toolByID.get(block.id);
						if (!tool) {
							if (!isStreaming) {
								return null;
							}
							// Streaming placeholder for not-yet-resolved tool.
							return (
								<Tool
									key={block.id}
									name="Tool"
									status="running"
									isError={false}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									subagentStatusOverrides={subagentStatusOverrides}
									mcpServers={mcpServers}
								/>
							);
						}
						return (
							<Tool
								key={tool.id}
								name={tool.name}
								args={tool.args}
								result={tool.result}
								status={tool.status}
								isError={tool.isError}
								killedBySignal={tool.killedBySignal}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								showDesktopPreviews={showDesktopPreviews}
								subagentStatusOverrides={
									isStreaming ? subagentStatusOverrides : undefined
								}
								mcpServerConfigId={tool.mcpServerConfigId}
								mcpServers={mcpServers}
								onImplementPlan={onImplementPlan}
								onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
								isChatCompleted={isChatCompleted}
								isLatestAskUserQuestion={
									tool.id === latestAskUserQuestionToolId &&
									!hasUserResponseAfterAskQuestion
								}
								previousResponseText={
									tool.name === "ask_user_question"
										? askUserQuestionResponseTextByToolId?.get(tool.id)
										: undefined
								}
								modelIntent={tool.modelIntent}
							/>
						);
					}
					case "file":
						return (
							<AttachmentBlock
								key={`${keyPrefix}-file-${block.file_id ?? index}`}
								block={block}
								onImageClick={onImageClick}
								onTextFileClick={onTextFileClick}
								framePreview
								showTextStatus
							/>
						);
					case "sources":
						return (
							<WebSearchSources
								key={`${keyPrefix}-sources-${index}`}
								sources={block.sources}
							/>
						);
					default:
						return null;
				}
			})}
			{remainingTools.map((tool) => (
				<Tool
					key={tool.id}
					name={tool.name}
					args={tool.args}
					result={tool.result}
					status={tool.status}
					isError={tool.isError}
					killedBySignal={tool.killedBySignal}
					subagentTitles={subagentTitles}
					subagentVariants={subagentVariants}
					showDesktopPreviews={showDesktopPreviews}
					subagentStatusOverrides={
						isStreaming ? subagentStatusOverrides : undefined
					}
					mcpServerConfigId={tool.mcpServerConfigId}
					mcpServers={mcpServers}
					onImplementPlan={onImplementPlan}
					onSendAskUserQuestionResponse={onSendAskUserQuestionResponse}
					isChatCompleted={isChatCompleted}
					isLatestAskUserQuestion={
						tool.id === latestAskUserQuestionToolId &&
						!hasUserResponseAfterAskQuestion
					}
					previousResponseText={
						tool.name === "ask_user_question"
							? askUserQuestionResponseTextByToolId?.get(tool.id)
							: undefined
					}
					modelIntent={tool.modelIntent}
				/>
			))}
		</>
	);
};

const ChatMessageItem = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	isAfterEditingMessage?: boolean;
	hideActions?: boolean;

	// When true, renders a gradient overlay inside the bubble
	// that fades text out toward the bottom. Used by the sticky
	// overlay to indicate truncated content.
	fadeFromBottom?: boolean;
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
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
		hideActions = false,
		fadeFromBottom = false,
		onImplementPlan,
		onSendAskUserQuestionResponse,
		isChatCompleted,
		latestAskUserQuestionToolId,
		askUserQuestionResponseTextByToolId,
		hasUserResponseAfterAskQuestion = false,

		urlTransform,
		mcpServers,
		subagentTitles,
		subagentVariants,
		showDesktopPreviews,
	}) => {
		const isUser = message.role === "user";
		const [previewImage, setPreviewImage] = useState<string | null>(null);
		const [previewText, setPreviewText] =
			useState<PreviewTextAttachment | null>(null);
		const displayState = deriveMessageDisplayState({
			message,
			parsed,
			hideActions,
		});
		if (displayState.shouldHide) {
			return null;
		}

		const hasRenderableContent =
			parsed.blocks.length > 0 ||
			parsed.tools.length > 0 ||
			parsed.sources.length > 0;
		const conversationItemProps: { role: "user" | "assistant" } = {
			role: isUser ? "user" : "assistant",
		};

		return (
			<div
				className={cn(
					isAfterEditingMessage && "opacity-40 pointer-events-none",
					"group/msg relative transition-opacity duration-200",
				)}
			>
				<ConversationItem {...conversationItemProps}>
					{isUser ? (
						<UserMessageContent
							displayState={displayState}
							markdown={parsed.markdown}
							isEditing={editingMessageId === message.id}
							fadeFromBottom={fadeFromBottom}
							onImageClick={setPreviewImage}
							onTextFileClick={setPreviewText}
						/>
					) : (
						<Message className="w-full">
							<MessageContent className="whitespace-normal">
								<div className="relative space-y-3 overflow-visible">
									<BlockList
										blocks={parsed.blocks}
										tools={parsed.tools}
										keyPrefix={String(message.id)}
										subagentTitles={subagentTitles}
										subagentVariants={subagentVariants}
										showDesktopPreviews={showDesktopPreviews}
										onImplementPlan={onImplementPlan}
										onSendAskUserQuestionResponse={
											onSendAskUserQuestionResponse
										}
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
									{!hasRenderableContent && (
										<div className="text-xs text-content-secondary">
											Message has no renderable content.
										</div>
									)}
								</div>
							</MessageContent>
						</Message>
					)}
				</ConversationItem>
				{!hideActions &&
					(displayState.hasCopyableContent ||
						(isUser && onEditUserMessage)) && (
						<div
							className="mt-0.5 flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100"
							data-testid="message-actions"
						>
							{displayState.hasCopyableContent && (
								<CopyButton
									text={parsed.markdown}
									label="Copy message"
									className="size-6"
									tooltipSide="bottom"
								/>
							)}
							{isUser && onEditUserMessage && (
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
												onEditUserMessage(message.id, text, fileBlocks);
											}}
										>
											<PencilIcon />
											<span className="sr-only">Edit message</span>
										</Button>
									</TooltipTrigger>
									<TooltipContent side="bottom">Edit message</TooltipContent>
								</Tooltip>
							)}
						</div>
					)}
				{displayState.needsAssistantBottomSpacer && (
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
						onClose={() => setPreviewText(null)}
					/>
				)}
			</div>
		);
	},
);

const StickyUserMessage = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	isAfterEditingMessage?: boolean;
	latestUserMessageId?: number;
	assistantBelowHeight: number;
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
		latestUserMessageId,
		assistantBelowHeight,
	}) => {
		const markerRef = useRef<HTMLDivElement>(null);
		const messageRef = useRef<HTMLDivElement>(null);
		const [previewMetrics, setPreviewMetrics] = useState(() =>
			getPinnedPreviewMetrics({
				messageHeight: 0,
				scrolledPast: 0,
			}),
		);

		const handleEditUserMessage = onEditUserMessage
			? (
					messageId: number,
					text: string,
					fileBlocks?: readonly TypesGen.ChatMessagePart[],
				) => {
					const markerElement = markerRef.current;
					const scroller = markerElement?.closest(
						"[data-testid='scroll-container']",
					) as HTMLElement | null;
					const offsetBeforeEdit =
						markerElement && scroller
							? markerElement.getBoundingClientRect().top -
								scroller.getBoundingClientRect().top
							: null;
					onEditUserMessage(messageId, text, fileBlocks);
					requestAnimationFrame(() => {
						const nextMarkerElement = markerRef.current;
						const nextScroller = nextMarkerElement?.closest(
							"[data-testid='scroll-container']",
						) as HTMLElement | null;
						if (
							offsetBeforeEdit === null ||
							!nextMarkerElement ||
							!nextScroller
						) {
							return;
						}
						const offsetAfterEdit =
							nextMarkerElement.getBoundingClientRect().top -
							nextScroller.getBoundingClientRect().top;
						const offsetDelta = offsetAfterEdit - offsetBeforeEdit;
						if (Math.abs(offsetDelta) <= 1) {
							return;
						}
						nextScroller.scrollBy({
							top: offsetDelta,
							behavior: "instant",
						});
					});
				}
			: undefined;

		useEffect(() => {
			const markerElement = markerRef.current;
			const messageElement = messageRef.current;
			const scroller = messageElement?.closest(
				"[data-testid='scroll-container']",
			) as HTMLElement | null;
			if (!markerElement || !messageElement || !scroller) {
				return;
			}

			if (
				latestUserMessageId !== message.id ||
				assistantBelowHeight < PINNED_PREVIEW_MIN_HEIGHT_PX
			) {
				setPreviewMetrics(
					getPinnedPreviewMetrics({
						messageHeight: messageElement.offsetHeight,
						scrolledPast: 0,
					}),
				);
				return;
			}

			const updatePinnedPreview = () => {
				const scrolledPast =
					scroller.getBoundingClientRect().top -
					markerElement.getBoundingClientRect().top -
					8;
				setPreviewMetrics(
					getPinnedPreviewMetrics({
						messageHeight: messageElement.offsetHeight,
						scrolledPast,
					}),
				);
			};

			let frameId: number | null = null;
			const scheduleUpdate = () => {
				if (frameId !== null) {
					return;
				}
				frameId = requestAnimationFrame(() => {
					frameId = null;
					updatePinnedPreview();
				});
			};

			const observer = new ResizeObserver(scheduleUpdate);
			observer.observe(messageElement);
			observer.observe(scroller);
			scroller.addEventListener("scroll", scheduleUpdate, { passive: true });
			scheduleUpdate();

			return () => {
				observer.disconnect();
				scroller.removeEventListener("scroll", scheduleUpdate);
				if (frameId !== null) {
					cancelAnimationFrame(frameId);
				}
			};
		}, [assistantBelowHeight, latestUserMessageId, message.id]);

		const showOverlay = previewMetrics.active;

		return (
			<>
				<div
					ref={markerRef}
					data-chat-anchor="true"
					data-chat-anchor-id={`message-${message.id}`}
					className="pointer-events-none h-px"
				/>
				<div
					ref={messageRef}
					className={cn(
						"relative px-3 -mx-3 -mt-2",
						showOverlay && "sticky top-2 z-10",
					)}
				>
					<div
						inert={showOverlay ? true : undefined}
						aria-hidden={showOverlay || undefined}
						className={
							showOverlay ? "pointer-events-none" : "pointer-events-auto"
						}
						style={
							showOverlay
								? { opacity: 1 - previewMetrics.overlayOpacity }
								: undefined
						}
					>
						<ChatMessageItem
							message={message}
							parsed={parsed}
							onEditUserMessage={handleEditUserMessage}
							editingMessageId={editingMessageId}
							isAfterEditingMessage={isAfterEditingMessage}
						/>
					</div>
					{showOverlay ? (
						<div
							data-chat-anchor-ignore="true"
							className="pointer-events-none absolute inset-x-0 top-0 z-10"
							style={{ opacity: previewMetrics.overlayOpacity }}
						>
							<div
								className="absolute inset-0 backdrop-blur-[1px] bg-surface-primary/15"
								style={{
									opacity: previewMetrics.fadeOpacity,
									maxHeight: `${previewMetrics.clipHeight + 48}px`,
									maskImage: `linear-gradient(to bottom, black ${previewMetrics.clipHeight + 24}px, transparent ${previewMetrics.clipHeight + 48}px)`,
									WebkitMaskImage: `linear-gradient(to bottom, black ${previewMetrics.clipHeight + 24}px, transparent ${previewMetrics.clipHeight + 48}px)`,
								}}
							/>
							<div className="relative px-3 pointer-events-auto">
								<ChatMessageItem
									message={message}
									parsed={parsed}
									onEditUserMessage={handleEditUserMessage}
									editingMessageId={editingMessageId}
									isAfterEditingMessage={isAfterEditingMessage}
									fadeFromBottom
								/>
							</div>
						</div>
					) : null}
				</div>
			</>
		);
	},
);

interface ConversationTimelineProps {
	parsedMessages: readonly ParsedMessageEntry[];
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
	hasLiveContentBelowLatestUser?: boolean;
}

export const ConversationTimeline = memo<ConversationTimelineProps>(
	({
		parsedMessages,
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
		hasLiveContentBelowLatestUser = false,
	}) => {
		if (parsedMessages.length === 0) {
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

		const latestUserMessageId = [...parsedMessages]
			.reverse()
			.find((entry) => entry.message.role === "user")?.message.id;
		const latestUserMessageIndex = latestUserMessageId
			? parsedMessages.findIndex(
					(entry) => entry.message.id === latestUserMessageId,
				)
			: -1;
		let assistantBelowHeight = 0;
		for (const entry of parsedMessages.slice(latestUserMessageIndex + 1)) {
			if (entry.message.role === "assistant") {
				assistantBelowHeight += 1;
			}
		}
		if (hasLiveContentBelowLatestUser) {
			assistantBelowHeight += 1;
		}

		return (
			<ExpiredFileIdsProvider>
				<div
					data-testid="conversation-timeline"
					className="flex flex-col gap-2"
				>
					{parsedMessages.map(({ message, parsed }, msgIdx) => {
						if (message.role === "user") {
							return (
								<StickyUserMessage
									key={message.id}
									message={message}
									parsed={parsed}
									onEditUserMessage={onEditUserMessage}
									editingMessageId={editingMessageId}
									isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
									latestUserMessageId={latestUserMessageId}
									assistantBelowHeight={assistantBelowHeight * 120}
								/>
							);
						}
						// Hide actions on assistant messages that are not
						// the last in a consecutive assistant chain.
						const next = parsedMessages[msgIdx + 1];
						const isLastInChain = !next || next.message.role === "user";
						return (
							<div
								key={message.id}
								data-chat-anchor="true"
								data-chat-anchor-id={`message-${message.id}`}
							>
								<ChatMessageItem
									message={message}
									parsed={parsed}
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
									isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
									hideActions={!isLastInChain}
									mcpServers={mcpServers}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									showDesktopPreviews={showDesktopPreviews}
								/>
							</div>
						);
					})}
				</div>
			</ExpiredFileIdsProvider>
		);
	},
);
