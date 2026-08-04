import {
	ChevronLeftIcon,
	ChevronRightIcon,
	InfoIcon,
	PencilIcon,
} from "lucide-react";
import {
	type FC,
	Fragment,
	memo,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";

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
	Tool,
} from "../ChatElements";
import { WebSearchSources } from "../ChatElements/tools";
import { ReadFilesTool } from "../ChatElements/tools/ReadFilesTool";
import {
	getReadFileToolData,
	ReadFileTool,
} from "../ChatElements/tools/ReadFileTool";
import type { SubagentVariant } from "../ChatElements/tools/subagentDescriptor";
import { ToolCall } from "../ChatElements/tools/ToolCall";
import { ImageLightbox } from "../ImageLightbox";
import { TextPreviewDialog } from "../TextPreviewDialog";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import { groupSequentialReadFileBlocks } from "./blockUtils";
import { FileProbeProvider } from "./FileProbeContext";
import {
	buildDisplayMessages,
	deriveMessageDisplayState,
} from "./messageHelpers";
import { getEditableUserMessagePayload } from "./messageParsing";
import { useSmoothStreamingText } from "./SmoothText";
import { getThinkingDisclosureDisplay } from "./thinkingTitle";
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
	thinkingDisplayMode?: ThinkingDisplayMode;
}>(
	({
		id,
		text,
		isStreaming = false,
		urlTransform,
		thinkingDisplayMode: mode = "auto",
	}) => {
		const [manualToggle, setManualToggle] = useState<boolean | null>(null);

		// Reset manual override on streaming transitions so
		// auto/preview modes collapse when streaming stops.
		const [prevStreaming, setPrevStreaming] = useState(isStreaming);
		if (prevStreaming !== isStreaming) {
			setPrevStreaming(isStreaming);
			if (mode === "auto" || mode === "preview") {
				setManualToggle(null);
			}
		}

		const autoExpanded = (() => {
			switch (mode) {
				case "always_expanded":
					return true;
				case "always_collapsed":
					return false;
				case "auto":
				case "preview":
					return isStreaming;
				default: {
					const _exhaustive: never = mode;
					return _exhaustive;
				}
			}
		})();

		const expanded = manualToggle ?? autoExpanded;

		const isPreviewConstrained =
			mode === "preview" && isStreaming && manualToggle === null;

		const previewScrollRef = useRef<HTMLDivElement>(null);

		const { visibleText } = useSmoothStreamingText({
			fullText: text,
			isStreaming,
			bypassSmoothing: !isStreaming,
			streamKey: id,
		});
		const displayText = isStreaming ? visibleText : text;
		const { title, body } = getThinkingDisclosureDisplay(displayText);
		const hasText = body.trim().length > 0;

		// Auto-scroll the preview container to the bottom as new
		// thinking content streams in. useLayoutEffect avoids a
		// visible frame where content has grown but not scrolled.
		const displayTextLength = body.length;
		useLayoutEffect(() => {
			if (
				displayTextLength &&
				isPreviewConstrained &&
				previewScrollRef.current
			) {
				previewScrollRef.current.scrollTop =
					previewScrollRef.current.scrollHeight;
			}
		}, [displayTextLength, isPreviewConstrained]);

		return (
			<div data-transcript-row="">
				<ToolCall.Root
					className="w-full"
					status={isStreaming ? "running" : "completed"}
					hasContent={hasText}
					expanded={expanded}
					onExpandedChange={(open) => setManualToggle(open)}
				>
					<ToolCall.Header
						iconName="thinking"
						label={title}
						showStatus={false}
					/>
					<ToolCall.Content>
						<div
							ref={previewScrollRef}
							className={cn(
								"mt-1.5",
								isPreviewConstrained && "max-h-24 overflow-y-auto",
							)}
						>
							<Response
								className="text-[11px] text-content-secondary"
								urlTransform={urlTransform}
								streaming={isStreaming}
							>
								{body}
							</Response>
						</div>
					</ToolCall.Content>
				</ToolCall.Root>
			</div>
		);
	},
);

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

const ReadFileTimelineBlock = memo<{
	tools: readonly [MergedTool, ...MergedTool[]];
}>(({ tools }) => {
	const [expanded, setExpanded] = useState(false);
	const [firstTool] = tools;

	if (tools.length === 1) {
		const readFile = getReadFileToolData(firstTool);
		return (
			<div data-tool-call="">
				<ReadFileTool
					{...readFile}
					status={firstTool.status}
					expanded={expanded}
					onExpandedChange={setExpanded}
				/>
			</div>
		);
	}

	return (
		<ReadFilesTool
			tools={tools}
			expanded={expanded}
			onExpandedChange={setExpanded}
		/>
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
	const prefQuery = useQuery(preferenceSettings());
	const thinkingDisplayMode: ThinkingDisplayMode =
		prefQuery.data?.thinking_display_mode || "auto";
	const shellToolDisplayMode: TypesGen.AgentDisplayMode =
		prefQuery.data?.shell_tool_display_mode || "always_collapsed";
	const codeDiffDisplayMode: TypesGen.AgentDisplayMode =
		prefQuery.data?.code_diff_display_mode || "auto";

	const toolByID = new Map(tools.map((tool) => [tool.id, tool]));
	const displayBlocks = groupSequentialReadFileBlocks(blocks, tools);

	// Pre-compute which tool IDs have a corresponding block so
	// we can render "remaining" (block-less) tools afterwards.
	const blockToolIDs = new Set(
		displayBlocks.flatMap((block) => {
			if (block.type === "tool") {
				return toolByID.has(block.id) || isStreaming ? [block.id] : [];
			}
			if (block.type === "tool-group") {
				return block.ids;
			}
			return [];
		}),
	);

	const remainingTools = tools.filter((tool) => !blockToolIDs.has(tool.id));

	// A thinking block is actively streaming only when it is the
	// very last block in the list. Once newer content arrives
	// (response, tool call, etc.) the thinking phase is over.
	const lastDisplayBlockIsThinking =
		displayBlocks.length > 0 &&
		displayBlocks[displayBlocks.length - 1].type === "thinking";

	return (
		<>
			{displayBlocks.map((block, index) => {
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
								isStreaming={
									isStreaming &&
									lastDisplayBlockIsThinking &&
									index === displayBlocks.length - 1
								}
								urlTransform={urlTransform}
								thinkingDisplayMode={thinkingDisplayMode}
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
					case "tool-group": {
						const [firstGroupTool, ...restGroupTools] = block.ids
							.map((id) => toolByID.get(id))
							.filter((tool) => tool !== undefined);
						if (!firstGroupTool) {
							return null;
						}
						return (
							<ReadFileTimelineBlock
								key={firstGroupTool.id}
								tools={[firstGroupTool, ...restGroupTools]}
							/>
						);
					}
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
									shellToolDisplayMode={shellToolDisplayMode}
									codeDiffDisplayMode={codeDiffDisplayMode}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									subagentStatusOverrides={subagentStatusOverrides}
									mcpServers={mcpServers}
								/>
							);
						}
						if (tool.name === "read_file") {
							return <ReadFileTimelineBlock key={tool.id} tools={[tool]} />;
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
								shellToolDisplayMode={shellToolDisplayMode}
								codeDiffDisplayMode={codeDiffDisplayMode}
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
								parsedCommands={tool.parsedCommands}
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
					default: {
						const _exhaustive: never = block;
						return _exhaustive;
					}
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
					shellToolDisplayMode={shellToolDisplayMode}
					codeDiffDisplayMode={codeDiffDisplayMode}
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
					parsedCommands={tool.parsedCommands}
				/>
			))}
		</>
	);
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
	hasActiveStream?: boolean;
	isAwaitingFirstStreamChunk?: boolean;

	// The bottom spacer fakes the height of the hidden action bar so
	// chain-end messages keep even spacing before the next bubble.
	// The last transcript message has nothing after it, so the spacer
	// would render as a dangling blank at the end of the chat.
	isLastMessage?: boolean;
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
	prevUserMessageId?: number;
	nextUserMessageId?: number;
	onJumpToUserMessage?: (messageId: number) => void;
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
		hideActions = false,
		hasActiveStream = false,
		isAwaitingFirstStreamChunk = false,
		isLastMessage = false,
		fadeFromBottom = false,
		onImplementPlan,
		onSendAskUserQuestionResponse,
		isChatCompleted,
		latestAskUserQuestionToolId,
		askUserQuestionResponseTextByToolId,
		hasUserResponseAfterAskQuestion = false,
		prevUserMessageId,
		nextUserMessageId,
		onJumpToUserMessage,

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
			hasActiveStream,
			isAwaitingFirstStreamChunk,
		});
		if (displayState.shouldHide) {
			return null;
		}
		if (message.role === "system") {
			return (
				<div
					className={cn(
						isAfterEditingMessage && "opacity-40 pointer-events-none",
						"transition-opacity duration-200",
					)}
					// Keep links in dimmed notices out of accessibility navigation.
					inert={isAfterEditingMessage ? true : undefined}
				>
					<LifecycleHookNotice urlTransform={urlTransform}>
						{parsed.markdown}
					</LifecycleHookNotice>
				</div>
			);
		}

		const conversationItemProps: { role: "user" | "assistant" } = {
			role: isUser ? "user" : "assistant",
		};

		return (
			<div
				className={cn(
					isAfterEditingMessage && "opacity-40 pointer-events-none",
					"group/msg relative transition-opacity duration-200",
				)}
				inert={isAfterEditingMessage ? true : undefined}
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
								{/* Keep assistant content spacing consistent by letting the parent stack own every top-level gap. */}
								<div className="relative flex flex-col gap-2 overflow-visible">
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
								</div>
							</MessageContent>
						</Message>
					)}
				</ConversationItem>
				{parsed.hookNotices.map((notice, index) => (
					<LifecycleHookNotice
						key={`${message.id}-hook-notice-${index}`}
						urlTransform={urlTransform}
					>
						{notice}
					</LifecycleHookNotice>
				))}
				{!hideActions &&
					(displayState.hasCopyableContent ||
						(isUser && onEditUserMessage)) && (
						<div
							className={cn(
								"mt-0.5 flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100",
								isUser && "w-full justify-end",
							)}
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
							{isUser &&
								onJumpToUserMessage &&
								(prevUserMessageId !== undefined ||
									nextUserMessageId !== undefined) && (
									<>
										<Tooltip>
											<TooltipTrigger asChild>
												<Button
													size="icon"
													variant="subtle"
													className="size-6"
													aria-label="Jump to previous user message"
													disabled={prevUserMessageId === undefined}
													onClick={() => {
														if (prevUserMessageId !== undefined) {
															onJumpToUserMessage(prevUserMessageId);
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
													disabled={nextUserMessageId === undefined}
													onClick={() => {
														if (nextUserMessageId !== undefined) {
															onJumpToUserMessage(nextUserMessageId);
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
				{displayState.needsAssistantBottomSpacer && !isLastMessage && (
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

const MIN_HEIGHT = 72;
const STICKY_TOP = 8;
const FADE_RANGE = 40;
// The gap between a turn's last message and the next turn's sentinel, from the
// timeline's `gap-2`. It sits between the pinned prompt's section edge and the
// following section, so it belongs in the slack that keeps native push-out on
// the same scroll offset as the visible bubble reaching the boundary.
const TURN_GAP = 8;

/**
 * A turn's prompt. The section around it is the sticky containing block, so the
 * browser owns pinning and push-out; the only thing measured here is how much
 * of the bubble is still visible.
 *
 * The sticky box is deliberately only as tall as the visible bubble plus its
 * action row, because native push-out is driven by that height. The clipped
 * remainder is reproduced by a spacer outside the sticky box, which keeps the
 * section, and therefore the scroll geometry, at its full height.
 */
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
	prevUserMessageId?: number;
	nextUserMessageId?: number;
	onJumpToUserMessage?: (messageId: number) => void;
	registerSentinel?: (messageId: number, el: HTMLDivElement | null) => void;
	urlTransform?: UrlTransform;
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
		prevUserMessageId,
		nextUserMessageId,
		onJumpToUserMessage,
		registerSentinel,
		urlTransform,
	}) => {
		const sentinelRef = useRef<HTMLDivElement>(null);
		const messageId = message.id;
		const setSentinelRef = (el: HTMLDivElement | null) => {
			sentinelRef.current = el;
			registerSentinel?.(messageId, el);
		};
		const containerRef = useRef<HTMLDivElement>(null);
		const spacerRef = useRef<HTMLDivElement>(null);

		// A layout effect, never a passive one: `ResizeObserver` delivers its
		// first callback after layout and before the first paint, so the clip is
		// correct on the frame the prompt first appears. Observing from a passive
		// effect paints one frame with an unclipped prompt.
		useLayoutEffect(() => {
			const sentinel = sentinelRef.current;
			const container = containerRef.current;
			const spacer = spacerRef.current;
			if (!sentinel || !container || !spacer) return;
			const scroller = sentinel.closest<HTMLElement>(".overflow-y-auto");
			if (!scroller) return;
			// The bubble carries `max-height`, so its own height reports the
			// clipped size. Its inner body is never clipped, which is the only
			// reliable source for the uncompressed height.
			const body = container.querySelector<HTMLElement>(
				"[data-turn-prompt-body]",
			);
			const bubble = body?.parentElement;
			if (!body || !bubble) return;

			// The clip shrinks only the bubble's content box. The bubble's own
			// padding and border, and the action row below it, keep their height,
			// so both are measured on resize rather than per frame.
			let bubbleChrome = 0;
			let rowHeight = 0;
			let bodyHeight = 0;
			let scrollerHeight = 0;
			const measure = () => {
				const bubbleStyle = getComputedStyle(bubble);
				bubbleChrome =
					Number.parseFloat(bubbleStyle.paddingTop) +
					Number.parseFloat(bubbleStyle.paddingBottom) +
					Number.parseFloat(bubbleStyle.borderTopWidth) +
					Number.parseFloat(bubbleStyle.borderBottomWidth);
				rowHeight = Math.max(
					container.getBoundingClientRect().height -
						bubble.getBoundingClientRect().height,
					0,
				);
				bodyHeight = body.getBoundingClientRect().height;
				scrollerHeight = scroller.clientHeight;
			};

			const update = () => {
				const scrollerTop = scroller.getBoundingClientRect().top;
				const sentinelTop = sentinel.getBoundingClientRect().top;
				const fullHeight = bodyHeight + bubbleChrome + rowHeight;
				// Compression is measured from the pin line, the same edge the
				// browser pins against, so compression starts exactly when
				// pinning does.
				const scrolledPast = Math.max(
					scrollerTop + STICKY_TOP - sentinelTop,
					0,
				);

				// Prompts taller than most of the scrollport are not worth
				// pinning: they would cover the reply they belong to.
				const tooTall = fullHeight > scrollerHeight * 0.75;
				container.style.position = tooTall ? "relative" : "";
				if (tooTall) {
					container.style.setProperty("--clip-h", `${fullHeight}px`);
					container.style.setProperty("--fade-opacity", "0");
					container.style.setProperty("--fade-display", "none");
					container.style.setProperty("--pin-slack", "0px");
					spacer.style.height = "0px";
					return;
				}

				// How much of the prompt is still shown. Never more than its own
				// height, so a prompt shorter than the floor is left alone.
				const visible = Math.min(
					fullHeight,
					Math.max(fullHeight - scrolledPast, MIN_HEIGHT),
				);
				container.style.setProperty("--clip-h", `${visible}px`);
				const fade =
					scrolledPast <= 0
						? 0
						: Math.max(
								0,
								Math.min((MIN_HEIGHT + FADE_RANGE - visible) / FADE_RANGE, 1),
							);
				container.style.setProperty("--fade-opacity", String(fade));
				// The frosted band carries `backdrop-filter` and a mask. Both promote
				// a compositing layer and open a stacking context even at zero
				// opacity, which changes how the text under the band is antialiased,
				// so it is kept out of rendering until the fade has something to
				// show.
				container.style.setProperty(
					"--fade-display",
					fade > 0 ? "block" : "none",
				);
				// `max-height` only shrinks the bubble, so the pinned box is the
				// clipped bubble plus the action row, and the browser pushes it out
				// when that box plus the gaps either side of the turn boundary
				// reach the boundary. Push-out should instead start when the
				// visible height reaches it, so the difference is given back as a
				// negative bottom margin and re-added by the spacer, leaving the
				// turn's total height untouched.
				const pinnedHeight =
					Math.min(bodyHeight + bubbleChrome, visible) + rowHeight;
				container.style.setProperty(
					"--pin-slack",
					`${pinnedHeight + 2 * TURN_GAP - visible}px`,
				);
				spacer.style.height = `${fullHeight - visible + 2 * TURN_GAP}px`;
			};

			// In `flex-col-reverse` the scroller stays at `scrollTop` 0 while
			// pinned to the bottom, so a growing transcript fires no scroll
			// event, and the growth changes neither the scroller's own box nor
			// the prompt body. The content wrapper is observed as well, so the
			// clip keeps up with a streaming reply.
			const content = sentinel.closest<HTMLElement>(
				"[data-chat-scroll-content]",
			);
			let rafId: number | null = null;
			const schedule = () => {
				if (rafId !== null) return;
				rafId = requestAnimationFrame(() => {
					rafId = null;
					update();
				});
			};
			const remeasure = () => {
				measure();
				schedule();
			};
			const observer = new ResizeObserver(remeasure);
			observer.observe(scroller);
			observer.observe(body);
			if (content) observer.observe(content);
			scroller.addEventListener("scroll", schedule, { passive: true });
			window.addEventListener("resize", remeasure);
			measure();
			update();
			return () => {
				scroller.removeEventListener("scroll", schedule);
				window.removeEventListener("resize", remeasure);
				observer.disconnect();
				if (rafId !== null) cancelAnimationFrame(rafId);
			};
		}, []);

		const handleEditUserMessage = onEditUserMessage
			? (
					messageId: number,
					text: string,
					fileBlocks?: readonly TypesGen.ChatMessagePart[],
				) => {
					onEditUserMessage(messageId, text, fileBlocks);
					// One frame later, so the edited message has been laid out.
					// `scroll-margin-top` on the sentinel lands it on the pin line.
					requestAnimationFrame(() => {
						sentinelRef.current?.scrollIntoView({
							behavior: "smooth",
							block: "start",
						});
					});
				}
			: undefined;

		return (
			<>
				{/* `scroll-mt-2` matches the 8px pin line, so both the jump
				    arrows and the post-edit scroll land the prompt exactly where
				    it comes to rest when pinned. */}
				<div
					ref={setSentinelRef}
					className="h-0 scroll-mt-2"
					data-user-sentinel
				/>
				<div
					ref={containerRef}
					className="sticky top-2 z-10 px-3 -mx-3 -mt-2"
					style={{ marginBottom: "calc(-1 * var(--pin-slack, 0px))" }}
					data-turn-prompt
				>
					{/* Frosted band below the clipped bubble. It reaches past the
					    sticky box on purpose, so the fade covers the transcript
					    scrolling underneath. Out of rendering until the fade has
					    something to show: the blur and mask promote a layer whether
					    or not the band is opaque. */}
					<div
						aria-hidden
						className="pointer-events-none absolute inset-x-0 top-0 backdrop-blur-[1px] bg-surface-primary/15"
						style={{
							display: "var(--fade-display, none)",
							opacity: "var(--fade-opacity, 0)",
							height: "calc(var(--clip-h, 0px) + 48px)",
							maskImage:
								"linear-gradient(to bottom, black calc(var(--clip-h, 100%) + 24px), transparent calc(var(--clip-h, 100%) + 48px))",
							WebkitMaskImage:
								"linear-gradient(to bottom, black calc(var(--clip-h, 100%) + 24px), transparent calc(var(--clip-h, 100%) + 48px))",
						}}
					/>
					<ChatMessageItem
						message={message}
						parsed={parsed}
						onEditUserMessage={handleEditUserMessage}
						editingMessageId={editingMessageId}
						isAfterEditingMessage={isAfterEditingMessage}
						prevUserMessageId={prevUserMessageId}
						nextUserMessageId={nextUserMessageId}
						onJumpToUserMessage={onJumpToUserMessage}
						urlTransform={urlTransform}
						fadeFromBottom
					/>
				</div>
				{/* Replaces the height the clip removes from the sticky box, so
				    the turn keeps its full height and the transcript below it
				    does not move while the prompt compresses. */}
				<div ref={spacerRef} aria-hidden className="-mt-2" />
			</>
		);
	},
);

function computeLastInChainFlags(
	displayMessages: readonly ParsedMessageEntry[],
): boolean[] {
	const flags = new Array<boolean>(displayMessages.length).fill(false);
	let nextVisibleIsUser = true;
	for (let i = displayMessages.length - 1; i >= 0; i--) {
		const entry = displayMessages[i];
		if (entry.message.role === "system") {
			nextVisibleIsUser = true;
			continue;
		}
		if (entry.message.role !== "user") {
			flags[i] = nextVisibleIsUser;
		}
		nextVisibleIsUser = entry.message.role === "user";
	}
	return flags;
}

type TimelineTurnEntry = {
	entry: ParsedMessageEntry;
	index: number;
};

type TimelineTurn = {
	key: string;
	prompt?: TimelineTurnEntry;
	items: TimelineTurnEntry[];
};

const LEADING_TURN_KEY = "turn-leading";

// A turn is one visible user prompt plus every message that follows it up to
// the next visible prompt. Hidden user messages carry context metadata only, so
// they never open a turn and the messages after them stay with the preceding
// prompt. A transcript page can start with assistant replies whose prompt lives
// on an older page that has not been fetched; those replies form a leading turn
// with no prompt and join the prompt's turn once that page arrives.
function groupDisplayMessagesIntoTurns(
	displayMessages: readonly ParsedMessageEntry[],
): TimelineTurn[] {
	const turns: TimelineTurn[] = [];
	let currentTurn: TimelineTurn | undefined;
	for (let index = 0; index < displayMessages.length; index++) {
		const entry = displayMessages[index];
		if (entry.message.role === "user") {
			const { shouldHide } = deriveMessageDisplayState({
				message: entry.message,
				parsed: entry.parsed,
				hideActions: false,
				hasActiveStream: false,
				isAwaitingFirstStreamChunk: false,
			});
			if (shouldHide) {
				continue;
			}
			currentTurn = {
				key: `turn-${entry.message.id}`,
				prompt: { entry, index },
				items: [],
			};
			turns.push(currentTurn);
			continue;
		}
		if (!currentTurn) {
			currentTurn = { key: LEADING_TURN_KEY, items: [] };
			turns.push(currentTurn);
		}
		currentTurn.items.push({ entry, index });
	}
	return turns;
}

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
	hasActiveStream?: boolean;
	isAwaitingFirstStreamChunk?: boolean;
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
		hasActiveStream,
		isAwaitingFirstStreamChunk,
	}) => {
		const sentinelsRef = useRef<Map<number, HTMLDivElement>>(new Map());
		const registerSentinel = (messageId: number, el: HTMLDivElement | null) => {
			if (el) {
				sentinelsRef.current.set(messageId, el);
			} else {
				sentinelsRef.current.delete(messageId);
			}
		};
		const jumpToUserMessage = (messageId: number) => {
			sentinelsRef.current.get(messageId)?.scrollIntoView({
				behavior: "smooth",
				block: "start",
			});
		};

		const displayMessages = buildDisplayMessages(parsedMessages);
		const lastInChainFlags = computeLastInChainFlags(displayMessages);
		const turns = groupDisplayMessagesIntoTurns(displayMessages);

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

		// Ordered list of visible user message IDs, used to drive the
		// per-bubble prev/next arrow buttons that jump the transcript
		// to the neighbouring user prompt, and to resolve the next
		// prompt's sentinel while a prompt is pinned. Turns already hold
		// exactly the prompts that render, in order.
		const visibleUserMessageIds = turns.flatMap((turn) =>
			turn.prompt ? [turn.prompt.entry.message.id] : [],
		);
		const userNeighborsById = new Map<
			number,
			{ prevId?: number; nextId?: number }
		>();
		for (let i = 0; i < visibleUserMessageIds.length; i++) {
			userNeighborsById.set(visibleUserMessageIds[i], {
				prevId: i > 0 ? visibleUserMessageIds[i - 1] : undefined,
				nextId:
					i < visibleUserMessageIds.length - 1
						? visibleUserMessageIds[i + 1]
						: undefined,
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
				<div
					data-testid="conversation-timeline"
					className="flex flex-col gap-2"
				>
					{turns.map((turn) => (
						// The section is the sticky containing block for its prompt, so
						// the browser pins the prompt and pushes it out at the turn
						// boundary without any scroll-driven position writes.
						<section
							key={turn.key}
							className="flex flex-col gap-2"
							data-chat-turn
						>
							{turn.prompt && (
								<StickyUserMessage
									message={turn.prompt.entry.message}
									parsed={turn.prompt.entry.parsed}
									onEditUserMessage={onEditUserMessage}
									editingMessageId={editingMessageId}
									isAfterEditingMessage={afterEditingMessageIds.has(
										turn.prompt.entry.message.id,
									)}
									prevUserMessageId={
										userNeighborsById.get(turn.prompt.entry.message.id)?.prevId
									}
									nextUserMessageId={
										userNeighborsById.get(turn.prompt.entry.message.id)?.nextId
									}
									onJumpToUserMessage={jumpToUserMessage}
									registerSentinel={registerSentinel}
									urlTransform={urlTransform}
								/>
							)}
							{turn.items.map(({ entry, index }) => (
								<ChatMessageItem
									key={entry.message.id}
									message={entry.message}
									parsed={entry.parsed}
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
									isAfterEditingMessage={afterEditingMessageIds.has(
										entry.message.id,
									)}
									// Hide actions on assistant messages that are not the
									// last in a consecutive assistant chain. Flags are
									// precomputed in a single reverse pass above.
									hideActions={!lastInChainFlags[index]}
									hasActiveStream={Boolean(hasActiveStream)}
									isAwaitingFirstStreamChunk={Boolean(
										isAwaitingFirstStreamChunk,
									)}
									isLastMessage={index === displayMessages.length - 1}
									mcpServers={mcpServers}
									subagentTitles={subagentTitles}
									subagentVariants={subagentVariants}
									showDesktopPreviews={showDesktopPreviews}
								/>
							))}
						</section>
					))}
				</div>
			</FileProbeProvider>
		);
	},
);
