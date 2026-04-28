import { ChevronDownIcon, PencilIcon } from "lucide-react";
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
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
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
		const hasText = displayText.trim().length > 0;

		// Auto-scroll the preview container to the bottom as new
		// thinking content streams in. useLayoutEffect avoids a
		// visible frame where content has grown but not scrolled.
		const displayTextLength = displayText.length;
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
			<div
				data-tool-call=""
				className={cn(
					"py-0.5",
					// Collapse padding between adjacent tool/thinking blocks.
					"[&:has(+[data-tool-call])]:pb-0",
					"[[data-tool-call]+&]:pt-0",
				)}
			>
				<Collapsible
					open={expanded}
					onOpenChange={(open) => setManualToggle(open)}
					className="w-full"
				>
					<CollapsibleTrigger
						className={cn(
							"border-0 bg-transparent p-0 m-0 font-[inherit] text-[inherit] text-left",
							"flex w-full items-center gap-2 cursor-pointer",
							"text-content-secondary transition-colors hover:text-content-primary",
						)}
					>
						{isStreaming ? (
							<Shimmer as="span" className="text-[13px]">
								Thinking
							</Shimmer>
						) : (
							<span className="text-[13px]">Thinking</span>
						)}
						<ChevronDownIcon
							className={cn(
								"h-3 w-3 shrink-0 text-current transition-transform",
								expanded ? "rotate-0" : "-rotate-90",
							)}
						/>
					</CollapsibleTrigger>
					{hasText && (
						<CollapsibleContent>
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
									{displayText}
								</Response>
							</div>
						</CollapsibleContent>
					)}
				</Collapsible>
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

	// A thinking block is actively streaming only when it is the
	// very last block in the list. Once newer content arrives
	// (response, tool call, etc.) the thinking phase is over.
	const lastBlockIsThinking =
		blocks.length > 0 && blocks[blocks.length - 1].type === "thinking";

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
								isStreaming={
									isStreaming &&
									lastBlockIsThinking &&
									index === blocks.length - 1
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
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		isAfterEditingMessage = false,
	}) => {
		const [isStuck, setIsStuck] = useState(false);
		const [isReady, setIsReady] = useState(false);
		const [isTooTall, setIsTooTall] = useState(false);
		const sentinelRef = useRef<HTMLDivElement>(null);
		const containerRef = useRef<HTMLDivElement>(null);
		const updateFnRef = useRef<(() => void) | null>(null);

		// useLayoutEffect so isStuck and --clip-h are both resolved
		// before the browser paints, avoiding a flash on load.
		useLayoutEffect(() => {
			const sentinel = sentinelRef.current;
			if (!sentinel) return;
			// Immediate check so the first paint is correct when the
			// sentinel is already scrolled out of view.
			const scroller = sentinel.closest(".overflow-y-auto");
			if (scroller) {
				const stuck =
					sentinel.getBoundingClientRect().top <
					scroller.getBoundingClientRect().top;
				if (stuck) {
					setIsStuck(true);
				}
			}
			setIsReady(true);
			const observer = new IntersectionObserver(
				([entry]) => setIsStuck(!entry.isIntersecting),
				{ threshold: 0 },
			);
			observer.observe(sentinel);
			return () => observer.disconnect();
		}, []);

		// Sets a single CSS custom property (--clip-h) on the sticky
		// container. All visual behaviour (max-height, mask fade) is
		// driven by CSS using this variable.
		useLayoutEffect(() => {
			const sentinel = sentinelRef.current;
			const container = containerRef.current;
			if (!sentinel || !container) return;
			const scroller = sentinel.closest(
				".overflow-y-auto",
			) as HTMLElement | null;
			if (!scroller) return;

			const MIN_HEIGHT = 72;
			const STICKY_TOP = 8;

			let scrollerTop = scroller.getBoundingClientRect().top;
			let scrollerHeight = scroller.clientHeight;

			const update = () => {
				const fullHeight = container.offsetHeight;

				// Skip sticky behavior for messages that take up
				// most of the visible area — accounting for the
				// chat input and some breathing room.
				const tooTall = fullHeight > scrollerHeight * 0.75;
				setIsTooTall(tooTall);
				if (tooTall) {
					container.style.setProperty("--clip-h", `${fullHeight}px`);
					container.style.setProperty("--fade-opacity", "0");
					container.style.top = `${STICKY_TOP}px`;

					return;
				}
				const sentinelTop = sentinel.getBoundingClientRect().top;
				const scrolledPast = scrollerTop - sentinelTop;

				if (scrolledPast <= 0) {
					// Always set a valid value so the overlay has the
					// correct height immediately when isStuck flips.
					container.style.setProperty("--clip-h", `${fullHeight}px`);
					container.style.setProperty("--fade-opacity", "0");
					container.style.top = `${STICKY_TOP}px`;

					return;
				}
				const visible = Math.max(fullHeight - scrolledPast, MIN_HEIGHT);
				container.style.setProperty("--clip-h", `${visible}px`);
				// Only show the blur and gradient once the message
				// is near its minimum compressed height. Ramp over
				// the last 40px before MIN_HEIGHT so it doesn't pop.
				const FADE_RANGE = 40;
				const fade = Math.max(
					0,
					Math.min((MIN_HEIGHT + FADE_RANGE - visible) / FADE_RANGE, 1),
				);
				container.style.setProperty("--fade-opacity", String(fade));
				// Push-up effect: when the next user message's sentinel
				// approaches the bottom of this sticky container, shift
				// this container upward so it slides out of view — the
				// same visual as the old section-boundary behavior.
				let nextSentinel: Element | null = sentinel.nextElementSibling;
				while (nextSentinel) {
					if (nextSentinel.hasAttribute("data-user-sentinel")) {
						break;
					}
					nextSentinel = nextSentinel.nextElementSibling;
				}
				if (nextSentinel) {
					const nextY = nextSentinel.getBoundingClientRect().top - scrollerTop;
					container.style.top = `${Math.min(STICKY_TOP, nextY - visible + STICKY_TOP)}px`;
				} else {
					container.style.top = `${STICKY_TOP}px`;
				}
			};
			updateFnRef.current = update;

			const onResize = () => {
				scrollerTop = scroller.getBoundingClientRect().top;
				scrollerHeight = scroller.clientHeight;
				update();
			};

			// Throttle to one update per animation frame so we don't
			// do redundant work on high-refresh-rate displays.
			let rafId: number | null = null;
			const onScroll = () => {
				if (rafId !== null) return;
				rafId = requestAnimationFrame(() => {
					rafId = null;
					update();
				});
			};

			// Re-run the visual update when the scrollable content height
			// changes (e.g. streaming responses growing the transcript).
			// In flex-col-reverse, scrollTop stays at 0 when pinned to
			// bottom so no scroll event fires — but the content wrapper
			// resizes and this observer catches that.
			const contentEl = scroller.firstElementChild as HTMLElement | null;
			let contentRafId: number | null = null;
			const contentObserver = contentEl
				? new ResizeObserver(() => {
						if (contentRafId !== null) return;
						contentRafId = requestAnimationFrame(() => {
							contentRafId = null;
							update();
						});
					})
				: null;
			contentObserver?.observe(contentEl!);

			scroller.addEventListener("scroll", onScroll, { passive: true });
			window.addEventListener("resize", onResize);
			update();
			// Set immediately — both --clip-h and --overlay-ready are
			// applied before the browser paints since we're in a
			// useLayoutEffect.
			container.style.setProperty("--overlay-ready", "1");
			return () => {
				scroller.removeEventListener("scroll", onScroll);
				window.removeEventListener("resize", onResize);
				contentObserver?.disconnect();
				container.style.removeProperty("--overlay-ready");
				if (rafId !== null) cancelAnimationFrame(rafId);
				if (contentRafId !== null) cancelAnimationFrame(contentRafId);
			};
		}, []);

		// Re-run the height calculation synchronously whenever
		// isStuck changes so --clip-h is correct on the same frame
		// the overlay appears. Without this, the async
		// IntersectionObserver + RAF-throttled scroll handler can
		// leave a stale --clip-h for one paint.
		// biome-ignore lint/correctness/useExhaustiveDependencies: isStuck is an intentional trigger
		useLayoutEffect(() => {
			updateFnRef.current?.();
		}, [isStuck]);

		const handleEditUserMessage = onEditUserMessage
			? (
					messageId: number,
					text: string,
					fileBlocks?: readonly TypesGen.ChatMessagePart[],
				) => {
					onEditUserMessage(messageId, text, fileBlocks);
					requestAnimationFrame(() => {
						const sentinel = sentinelRef.current;
						if (!sentinel) return;
						const scroller = sentinel.closest(
							".overflow-y-auto",
						) as HTMLElement | null;
						if (!scroller) return;
						const offset =
							sentinel.getBoundingClientRect().top -
							scroller.getBoundingClientRect().top;
						scroller.scrollBy({ top: offset, behavior: "smooth" });
					});
				}
			: undefined;

		return (
			<>
				<div ref={sentinelRef} className="h-0" data-user-sentinel />
				<div
					ref={containerRef}
					className={cn(
						"relative px-3 -mx-3 -mt-2",
						!isTooTall && "sticky z-10",
						!isReady && "invisible",
						isStuck && !isTooTall && "pointer-events-none",
					)}
				>
					{/* Flow element: always in the DOM to preserve
				    scroll layout. Hidden when stuck so the
				    clipped overlay takes over visually. */}
					<div
						className={
							isStuck && !isTooTall ? undefined : "pointer-events-auto"
						}
						style={
							isStuck && !isTooTall
								? { opacity: "calc(1 - var(--overlay-ready, 0))" }
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

					{/* Overlay: absolutely positioned, matching the
				    sticky container. max-height + mask are driven
				    entirely by the --clip-h CSS variable which the
				    scroll handler sets on the container. */}
					{isStuck && !isTooTall && (
						<div
							className="absolute inset-0"
							style={{
								opacity: "var(--overlay-ready, 0)",
								contain: "layout style",
							}}
						>
							{/* Blur layer: extends 48px beyond the
						    clipped content so the frosted effect
						    is visible around the bubble. Promoted
						    to its own GPU layer via will-change. */}
							<div
								className="absolute inset-0 backdrop-blur-[1px] bg-surface-primary/15"
								style={{
									opacity: "var(--fade-opacity, 0)",
									maxHeight: "calc(var(--clip-h, 100%) + 48px)",
									willChange: "max-height, mask-image",
									maskImage:
										"linear-gradient(to bottom, black calc(var(--clip-h, 100%) + 24px), transparent calc(var(--clip-h, 100%) + 48px))",
									WebkitMaskImage:
										"linear-gradient(to bottom, black calc(var(--clip-h, 100%) + 24px), transparent calc(var(--clip-h, 100%) + 48px))",
								}}
							/>
							{/* Content layer: px-3 matches the sticky
							    container's padding so the overlay aligns
							    with the flow element. will-change promotes
							    to GPU layer. */}
							<div className="relative px-3 pointer-events-auto will-change-[max-height]">
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
					)}
				</div>
			</>
		);
	},
);

function computeLastInChainFlags(
	parsedMessages: readonly ParsedMessageEntry[],
): boolean[] {
	const flags = new Array<boolean>(parsedMessages.length).fill(false);
	let nextVisibleIsUser = true; // no next visible => treat as chain end
	for (let i = parsedMessages.length - 1; i >= 0; i--) {
		const entry = parsedMessages[i];
		const { shouldHide } = deriveMessageDisplayState({
			message: entry.message,
			parsed: entry.parsed,
			hideActions: false,
		});
		if (entry.message.role !== "user") {
			flags[i] = nextVisibleIsUser;
		}
		if (!shouldHide) {
			nextVisibleIsUser = entry.message.role === "user";
		}
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
	isTurnActive?: boolean;
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
	}) => {
		const lastInChainFlags = computeLastInChainFlags(parsedMessages);

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
								/>
							);
						}
						// Hide actions on assistant messages that are not the
						// last in a consecutive assistant chain. Flags are
						// precomputed in a single reverse pass above.
						const isLastInChain = lastInChainFlags[msgIdx];
						return (
							<ChatMessageItem
								key={message.id}
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
						);
					})}
				</div>
			</ExpiredFileIdsProvider>
		);
	},
);
