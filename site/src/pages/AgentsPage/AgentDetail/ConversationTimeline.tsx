import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import {
	ConversationItem,
	Message,
	MessageContent,
	Response,
	Shimmer,
	Tool,
} from "components/ai-elements";
import { WebSearchSources } from "components/ai-elements/tool";
import { Button } from "components/Button/Button";
import { FileReferenceChip } from "components/ChatMessageInput/FileReferenceNode";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { PencilIcon } from "lucide-react";
import {
	type FC,
	Fragment,
	memo,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "utils/cn";
import { ImageThumbnail } from "../AgentChatInput";
import { ImageLightbox } from "../ImageLightbox";
import type { ChatDetailError } from "../usageLimitMessage";
import { useSmoothStreamingText } from "./SmoothText";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageEntry,
	RenderBlock,
	StreamState,
} from "./types";

const ReasoningDisclosure: FC<{
	id: string;
	text: string;
	isStreaming?: boolean;
	urlTransform?: UrlTransform;
}> = ({ id, text, isStreaming = false, urlTransform }) => {
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
};

// Shared block renderer used by both ChatMessageItem (historical
// messages) and StreamingOutput (live stream). Encapsulates the
// response / thinking / tool switch so the two consumers stay in sync.
type RenderBlockListParams = {
	blocks: readonly RenderBlock[];
	toolByID: ReadonlyMap<string, MergedTool>;
	keyPrefix: string;
	isStreaming?: boolean;
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	onImageClick?: (src: string) => void;
	urlTransform?: UrlTransform;
};

// Wrapper that runs the smooth-streaming jitter buffer on a single
// response block. Only used during live streaming — historical
// messages render through <Response> directly.
const SmoothedResponse: FC<{
	text: string;
	streamKey: string;
	urlTransform?: UrlTransform;
}> = ({ text, streamKey, urlTransform }) => {
	const { visibleText } = useSmoothStreamingText({
		fullText: text,
		isStreaming: true,
		bypassSmoothing: false,
		streamKey,
	});
	return <Response urlTransform={urlTransform}>{visibleText}</Response>;
};

type RenderBlockListResult = {
	elements: ReactNode[];
	renderedToolIDs: ReadonlySet<string>;
};

function renderBlockList({
	blocks,
	toolByID,
	keyPrefix,
	isStreaming = false,
	subagentTitles,
	subagentStatusOverrides,
	onImageClick,
	urlTransform,
}: RenderBlockListParams): RenderBlockListResult {
	const renderedToolIDs = new Set<string>();
	const elements = blocks
		.map((block, index) => {
			switch (block.type) {
				case "response":
					return isStreaming ? (
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
						renderedToolIDs.add(block.id);
						return (
							<Tool
								key={block.id}
								name="Tool"
								status="running"
								isError={false}
								subagentTitles={subagentTitles}
								subagentStatusOverrides={subagentStatusOverrides}
							/>
						);
					}
					renderedToolIDs.add(tool.id);
					return (
						<Tool
							key={tool.id}
							name={tool.name}
							args={tool.args}
							result={tool.result}
							status={tool.status}
							isError={tool.isError}
							subagentTitles={isStreaming ? subagentTitles : undefined}
							subagentStatusOverrides={
								isStreaming ? subagentStatusOverrides : undefined
							}
						/>
					);
				}
				case "file":
					if (block.media_type.startsWith("image/")) {
						const src = block.file_id
							? `/api/experimental/chats/files/${block.file_id}`
							: `data:${block.media_type};base64,${block.data}`;
						return (
							<button
								key={`${keyPrefix}-file-${index}`}
								type="button"
								aria-label="View image"
								className="inline-block rounded-md border-0 bg-transparent p-0"
								onClick={(e) => {
									e.stopPropagation();
									onImageClick?.(src);
								}}
							>
								<ImageThumbnail
									previewUrl={src}
									name="Attached image"
									className="cursor-pointer transition-opacity hover:opacity-80"
								/>
							</button>
						);
					}
					return null;
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
		})
		.filter((el): el is NonNullable<typeof el> => el != null);
	return { elements, renderedToolIDs };
}

const ChatMessageItem = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	isAfterEditingMessage?: boolean;
	// When true, renders a gradient overlay inside the bubble
	// that fades text out toward the bottom. Used by the sticky
	// overlay to indicate truncated content.
	fadeFromBottom?: boolean;
	urlTransform?: UrlTransform;
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		savingMessageId,
		isAfterEditingMessage = false,
		fadeFromBottom = false,
		urlTransform,
	}) => {
		const isUser = message.role === "user";
		const isSavingMessage = savingMessageId === message.id;
		const [previewImage, setPreviewImage] = useState<string | null>(null);
		const toolByID = new Map(parsed.tools.map((tool) => [tool.id, tool]));

		if (
			parsed.toolResults.length > 0 &&
			parsed.toolCalls.length === 0 &&
			parsed.markdown === "" &&
			parsed.reasoning === ""
		) {
			return null;
		}

		// Hide messages that consist entirely of provider-executed
		// tool results. The parser skips these parts, so the parsed
		// output is empty and would show a "no renderable content"
		// fallback.
		const parts = message.content ?? [];
		if (
			parts.length > 0 &&
			parts.every((p) => p.type === "tool-result" && p.provider_executed)
		) {
			return null;
		}

		const hasRenderableContent =
			parsed.blocks.length > 0 ||
			parsed.tools.length > 0 ||
			parsed.sources.length > 0;
		// Pre-compute the inline content for user messages so we
		// avoid a filter + map inside the JSX return path.
		const userInlineContent = isUser
			? parsed.blocks.filter(
					(
						b,
					): b is
						| Extract<RenderBlock, { type: "response" }>
						| Extract<RenderBlock, { type: "file-reference" }> =>
						b.type === "response" || b.type === "file-reference",
				)
			: [];

		const conversationItemProps: { role: "user" | "assistant" } = {
			role: isUser ? "user" : "assistant",
		};
		const { elements: orderedBlocks, renderedToolIDs } = renderBlockList({
			blocks: parsed.blocks,
			toolByID,
			keyPrefix: String(message.id),
			onImageClick: setPreviewImage,
			urlTransform,
		});
		const remainingTools = parsed.tools.filter(
			(tool) => !renderedToolIDs.has(tool.id),
		);

		return (
			<div
				className={cn(
					isAfterEditingMessage && "opacity-40 pointer-events-none",
					"transition-opacity duration-200",
				)}
			>
				<ConversationItem {...conversationItemProps}>
					{isUser ? (
						<Message className="w-full max-w-none">
							<MessageContent
								className={cn(
									"group/msg rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm transition-shadow",
									editingMessageId === message.id &&
										"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
									isSavingMessage && "ring-2 ring-content-secondary/40",
									fadeFromBottom && "relative overflow-hidden",
								)}
								style={
									fadeFromBottom
										? { maxHeight: "var(--clip-h, none)" }
										: undefined
								}
							>
								<div className="flex flex-col gap-1.5">
									<div className="flex items-start gap-2">
										<span className="min-w-0 flex-1">
											{userInlineContent.length > 0
												? userInlineContent.map((block, i) =>
														block.type === "response" ? (
															<Fragment key={i}>{block.text}</Fragment>
														) : (
															<FileReferenceChip
																key={i}
																fileName={block.file_name}
																startLine={block.start_line}
																endLine={block.end_line}
															/>
														),
													)
												: parsed.markdown || ""}
										</span>
										{isSavingMessage && (
											<Spinner
												className="mt-0.5 h-3.5 w-3.5 shrink-0 text-content-secondary"
												aria-label="Saving message edit"
												loading
											/>
										)}
										{onEditUserMessage && !isSavingMessage && (
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														type="button"
														className="mt-0.5 inline-flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md border-none bg-transparent p-0 text-content-secondary opacity-0 transition-opacity hover:bg-surface-tertiary hover:text-content-primary focus-visible:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link group-hover/msg:opacity-100"
														aria-label="Edit message"
														onClick={() => {
															const fileBlocks = parsed.blocks.filter(
																(
																	b,
																): b is Extract<
																	RenderBlock,
																	{ type: "file" }
																> =>
																	b.type === "file" &&
																	b.media_type.startsWith("image/"),
															);
															onEditUserMessage(
																message.id,
																parsed.markdown || "",
																fileBlocks.length > 0 ? fileBlocks : undefined,
															);
														}}
													>
														<PencilIcon className="size-3.5" />
													</button>
												</TooltipTrigger>
												<TooltipContent side="top">Edit message</TooltipContent>
											</Tooltip>
										)}
									</div>
									{(() => {
										const imageBlocks = parsed.blocks.filter(
											(b): b is Extract<RenderBlock, { type: "file" }> =>
												b.type === "file" && b.media_type.startsWith("image/"),
										);
										if (imageBlocks.length === 0) return null;
										return (
											<div className="mt-2 flex flex-wrap gap-2">
												{imageBlocks.map((block, i) => {
													const src = block.file_id
														? `/api/experimental/chats/files/${block.file_id}`
														: `data:${block.media_type};base64,${block.data}`;
													return (
														<button
															key={`user-file-${i}`}
															type="button"
															className="inline-block rounded-md border-0 bg-transparent p-0"
															onClick={(e) => {
																e.stopPropagation();
																setPreviewImage(src);
															}}
														>
															<ImageThumbnail
																previewUrl={src}
																name="Attached image"
																className="cursor-pointer transition-opacity hover:opacity-80"
															/>
														</button>
													);
												})}
											</div>
										);
									})()}
									{fadeFromBottom && (
										<div
											className="pointer-events-none absolute inset-x-0 bottom-0 h-1/2 max-h-12"
											style={{
												background:
													"linear-gradient(to top, hsl(var(--surface-secondary)), transparent)",
											}}
										/>
									)}
								</div>
							</MessageContent>
						</Message>
					) : (
						<Message className="w-full">
							<MessageContent className="whitespace-normal">
								<div className="space-y-3">
									{orderedBlocks}
									{remainingTools.map((tool) => (
										<Tool
											key={tool.id}
											name={tool.name}
											args={tool.args}
											result={tool.result}
											status={tool.status}
											isError={tool.isError}
										/>
									))}
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
				{previewImage && (
					<ImageLightbox
						src={previewImage}
						onClose={() => setPreviewImage(null)}
					/>
				)}
			</div>
		);
	},
);
ChatMessageItem.displayName = "ChatMessageItem";

export const StreamingOutput = memo<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	showInitialPlaceholder?: boolean;
	retryState?: { attempt: number; error: string } | null;
	urlTransform?: UrlTransform;
}>(
	({
		streamState,
		streamTools,
		subagentTitles,
		subagentStatusOverrides,
		showInitialPlaceholder = false,
		retryState,
		urlTransform,
	}) => {
		const conversationItemProps = { role: "assistant" as const };
		const toolByID = new Map(streamTools.map((tool) => [tool.id, tool]));
		const blocks = streamState?.blocks ?? [];
		const { elements: orderedBlocks, renderedToolIDs } = renderBlockList({
			blocks,
			toolByID,
			keyPrefix: "stream",
			isStreaming: true,
			subagentTitles,
			subagentStatusOverrides,
			urlTransform,
		});
		const remainingTools = streamTools.filter(
			(tool) => !renderedToolIDs.has(tool.id),
		);

		return (
			<ConversationItem {...conversationItemProps}>
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div className="space-y-3">
							{orderedBlocks}
							{showInitialPlaceholder ||
							(streamState &&
								orderedBlocks.length === 0 &&
								streamTools.length === 0) ? (
								<div className="relative">
									<Response aria-hidden className="invisible">
										{`Thinking...${retryState ? ` attempt ${retryState.attempt}` : ""}`}
									</Response>
									<div className="pointer-events-none absolute inset-0 flex items-baseline gap-2">
										<Shimmer as="div" className="text-[13px] leading-relaxed">
											Thinking...
										</Shimmer>
										{retryState && (
											<span className="text-[11px] text-content-secondary">
												attempt {retryState.attempt}
											</span>
										)}
									</div>
								</div>
							) : null}
							{remainingTools.map((tool) => (
								<Tool
									key={tool.id}
									name={tool.name}
									args={tool.args}
									result={tool.result}
									status={tool.status}
									isError={tool.isError}
									subagentTitles={subagentTitles}
									subagentStatusOverrides={subagentStatusOverrides}
								/>
							))}
						</div>
					</MessageContent>
				</Message>
			</ConversationItem>
		);
	},
);
StreamingOutput.displayName = "StreamingOutput";

const StickyUserMessage: FC<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	isAfterEditingMessage?: boolean;
}> = ({
	message,
	parsed,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
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
		const scroller = sentinel.closest(".overflow-y-auto") as HTMLElement | null;
		if (!scroller) return;

		const MIN_HEIGHT = 72;
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
				container.style.top = "0px";
				return;
			}
			const sentinelTop = sentinel.getBoundingClientRect().top;
			const scrolledPast = scrollerTop - sentinelTop;

			if (scrolledPast <= 0) {
				// Always set a valid value so the overlay has the
				// correct height immediately when isStuck flips.
				container.style.setProperty("--clip-h", `${fullHeight}px`);
				container.style.setProperty("--fade-opacity", "0");
				container.style.top = "0px";
				return;
			}
			const visible = Math.max(fullHeight - scrolledPast - 48, MIN_HEIGHT);
			container.style.setProperty("--clip-h", `${visible}px`);
			// Only show the fade gradient once enough content is
			// clipped to be visually meaningful.
			container.style.setProperty(
				"--fade-opacity",
				visible < fullHeight - 8 ? "1" : "0",
			);

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
				container.style.top = `${Math.min(0, nextY - visible)}px`;
			} else {
				container.style.top = "0px";
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
			container.style.removeProperty("--overlay-ready");
			if (rafId !== null) cancelAnimationFrame(rafId);
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
					"relative px-3 -mx-3 -mt-3",
					!isTooTall && "sticky z-10",
					!isReady && "invisible",
					isStuck && !isTooTall && "pointer-events-none",
				)}
			>
				{/* Flow element: always in the DOM to preserve
				    scroll layout. Hidden when stuck so the
				    clipped overlay takes over visually. */}
				<div
					className={isStuck && !isTooTall ? undefined : "pointer-events-auto"}
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
						savingMessageId={savingMessageId}
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
								savingMessageId={savingMessageId}
								isAfterEditingMessage={isAfterEditingMessage}
								fadeFromBottom
							/>
						</div>
					</div>
				)}
			</div>
		</>
	);
};

interface ConversationTimelineProps {
	isEmpty: boolean;
	parsedMessages: readonly ParsedMessageEntry[];
	hasStreamOutput: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles: Map<string, string>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	retryState?: { attempt: number; error: string } | null;
	isAwaitingFirstStreamChunk: boolean;
	detailError?: ChatDetailError | null;
	onOpenAnalytics?: () => void;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
	) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	urlTransform?: UrlTransform;
}

export const ConversationTimeline: FC<ConversationTimelineProps> = ({
	isEmpty,
	parsedMessages,
	hasStreamOutput,
	streamState,
	streamTools,
	subagentTitles,
	subagentStatusOverrides,
	retryState,
	isAwaitingFirstStreamChunk,
	detailError,
	onOpenAnalytics,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
}) => {
	const shouldRenderStreamAfterMessages =
		hasStreamOutput && parsedMessages.length > 0;
	const isUsageLimitError = detailError?.kind === "usage-limit";
	const showUsageAction = onOpenAnalytics !== undefined && isUsageLimitError;

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

	return (
		<div className="mx-auto w-full max-w-3xl py-6">
			{isEmpty && !hasStreamOutput ? (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			) : (
				<div className="flex flex-col gap-3">
					{parsedMessages.map(({ message, parsed }) =>
						message.role === "user" ? (
							<StickyUserMessage
								key={message.id}
								message={message}
								parsed={parsed}
								onEditUserMessage={onEditUserMessage}
								editingMessageId={editingMessageId}
								savingMessageId={savingMessageId}
								isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
							/>
						) : (
							<ChatMessageItem
								key={message.id}
								message={message}
								parsed={parsed}
								savingMessageId={savingMessageId}
								urlTransform={urlTransform}
								isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
							/>
						),
					)}
					{shouldRenderStreamAfterMessages && (
						<StreamingOutput
							streamState={streamState}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentStatusOverrides={subagentStatusOverrides}
							showInitialPlaceholder={isAwaitingFirstStreamChunk}
							retryState={retryState}
							urlTransform={urlTransform}
						/>
					)}
					{hasStreamOutput && parsedMessages.length === 0 && (
						<StreamingOutput
							streamState={streamState}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentStatusOverrides={subagentStatusOverrides}
							showInitialPlaceholder={isAwaitingFirstStreamChunk}
							retryState={retryState}
							urlTransform={urlTransform}
						/>
					)}
				</div>
			)}
			{detailError && (
				<Alert
					severity={isUsageLimitError ? "info" : "error"}
					className="py-2"
					actions={
						showUsageAction && (
							<Button variant="subtle" size="sm" onClick={onOpenAnalytics}>
								View Usage
							</Button>
						)
					}
				>
					{detailError.message}
				</Alert>
			)}
		</div>
	);
};
