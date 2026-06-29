import {
	MessageScroller,
	useMessageScroller,
} from "@shadcn/react/message-scroller";
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "lucide-react";
import {
	type FC,
	Fragment,
	memo,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

import { useQuery } from "react-query";
import type { UrlTransform } from "streamdown";
import { preferenceSettings } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";

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
	tools: readonly MergedTool[];
}>(({ tools }) => {
	const [expanded, setExpanded] = useState(false);
	const [firstTool] = tools;
	if (!firstTool) {
		return null;
	}

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
						const groupTools = block.ids
							.map((id) => toolByID.get(id))
							.filter((tool) => tool !== undefined);
						const [firstGroupTool] = groupTools;
						if (!firstGroupTool) {
							return null;
						}
						return (
							<ReadFileTimelineBlock
								key={firstGroupTool.id}
								tools={groupTools}
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

function computeLastInChainFlags(
	displayMessages: readonly ParsedMessageEntry[],
): boolean[] {
	const flags = new Array<boolean>(displayMessages.length).fill(false);
	let nextVisibleIsUser = true;
	for (let i = displayMessages.length - 1; i >= 0; i--) {
		const entry = displayMessages[i];
		if (entry.message.role !== "user") {
			flags[i] = nextVisibleIsUser;
		}
		nextVisibleIsUser = entry.message.role === "user";
	}
	return flags;
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
		const { scrollToMessage } = useMessageScroller();
		const jumpToUserMessage = (messageId: number) => {
			scrollToMessage(String(messageId), {
				align: "start",
				behavior: "smooth",
			});
		};

		const displayMessages = buildDisplayMessages(parsedMessages);
		const lastInChainFlags = computeLastInChainFlags(displayMessages);

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
		// to the neighbouring user prompt.
		const visibleUserMessageIds: number[] = [];
		for (const { message, parsed } of parsedMessages) {
			if (message.role !== "user") continue;
			const { shouldHide } = deriveMessageDisplayState({
				message,
				parsed,
				hideActions: false,
				hasActiveStream: false,
				isAwaitingFirstStreamChunk: false,
			});
			if (!shouldHide) visibleUserMessageIds.push(message.id);
		}
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
				{displayMessages.map(({ message, parsed }, msgIdx) => {
					if (message.role === "user") {
						const { shouldHide } = deriveMessageDisplayState({
							message,
							parsed,
							hideActions: false,
							hasActiveStream: false,
							isAwaitingFirstStreamChunk: false,
						});
						if (shouldHide) {
							return null;
						}
						return (
							<MessageScroller.Item
								key={message.id}
								messageId={String(message.id)}
								scrollAnchor
							>
								<ChatMessageItem
									message={message}
									parsed={parsed}
									onEditUserMessage={onEditUserMessage}
									editingMessageId={editingMessageId}
									isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
									prevUserMessageId={userNeighborsById.get(message.id)?.prevId}
									nextUserMessageId={userNeighborsById.get(message.id)?.nextId}
									onJumpToUserMessage={jumpToUserMessage}
								/>
							</MessageScroller.Item>
						);
					}
					// Hide actions on assistant messages that are not the
					// last in a consecutive assistant chain. Flags are
					// precomputed in a single reverse pass above.
					const isLastInChain = lastInChainFlags[msgIdx];
					return (
						<MessageScroller.Item
							key={message.id}
							messageId={String(message.id)}
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
								hasActiveStream={Boolean(hasActiveStream)}
								isAwaitingFirstStreamChunk={Boolean(isAwaitingFirstStreamChunk)}
								isLastMessage={msgIdx === displayMessages.length - 1}
								mcpServers={mcpServers}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								showDesktopPreviews={showDesktopPreviews}
							/>
						</MessageScroller.Item>
					);
				})}
			</FileProbeProvider>
		);
	},
);
