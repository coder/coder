import { API } from "api/api";
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
import { WorkingBlock } from "components/ai-elements/WorkingBlock";
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
	useMemo,
	useRef,
	useState,
} from "react";
import { Link } from "react-router";
import type { UrlTransform } from "streamdown";
import { cn } from "utils/cn";
import type { ChatDetailError } from "../../utils/usageLimitMessage";
import { ImageThumbnail } from "../AgentChatInput";
import { ImageLightbox } from "../ImageLightbox";
import { useSmoothStreamingText } from "./SmoothText";
import { parseMessagesWithMergedTools } from "./messageParsing";
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
	hideToolBlocks?: boolean;
	animateText?: boolean;
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
		hideToolBlocks = false,
		animateText = false,
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
			blocks: hideToolBlocks
				? parsed.blocks.filter(
						(b) => b.type === "response" || b.type === "thinking",
					)
				: parsed.blocks,
			toolByID,
			keyPrefix: String(message.id),
			isStreaming: animateText,
			onImageClick: setPreviewImage,
			urlTransform,
		});
		const remainingTools = hideToolBlocks
			? []
			: parsed.tools.filter((tool) => !renderedToolIDs.has(tool.id));

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
																className="mx-1"
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

export const StreamingOutput: FC<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	showInitialPlaceholder?: boolean;
	retryState?: { attempt: number; error: string } | null;
	urlTransform?: UrlTransform;
}> = ({
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
};

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
	chatId: string;
	isEmpty: boolean;
	parsedMessages: readonly ParsedMessageEntry[];
	workingBlocks?: readonly TypesGen.ChatUIWorkingBlock[];
	chatStatus?: TypesGen.ChatStatus;
	hasStreamOutput: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles: Map<string, string>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	streamHasToolCalls?: boolean;
	retryState?: { attempt: number; error: string } | null;
	isAwaitingFirstStreamChunk: boolean;
	detailError?: ChatDetailError | null;
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
	chatId,
	isEmpty,
	parsedMessages,
	workingBlocks,
	chatStatus,
	hasStreamOutput,
	streamState,
	streamTools,
	subagentTitles,
	subagentStatusOverrides,
	streamHasToolCalls,
	retryState,
	isAwaitingFirstStreamChunk,
	detailError,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
	urlTransform,
}) => {
	const shouldRenderStreamAfterMessages =
		hasStreamOutput && parsedMessages.length > 0;
	const isUsageLimitError = detailError?.kind === "usage-limit";

	// Track when the stream first enters the working phase so the
	// WorkingBlock timer starts from the right moment.
	const workingStartRef = useRef<string | null>(null);
	if (streamHasToolCalls && !workingStartRef.current) {
		workingStartRef.current = new Date().toISOString();
	}
	if (!streamHasToolCalls) {
		workingStartRef.current = null;
	}

	// Identify the last user message index once, shared by both
	// workingBlockAfterMessage and the live-stream hiding logic.
	const lastUserMsgIndex = (() => {
		for (let i = parsedMessages.length - 1; i >= 0; i--) {
			if (parsedMessages[i].message.role === "user") return i;
		}
		return -1;
	})();

	// Map working blocks by the message ID they follow.
	// Includes REST-provided blocks for historical turns and a
	// synthetic block for the current turn when its messages are
	// already in the store (from WebSocket durable events) but
	// the REST data hasn't refreshed yet.
	const workingBlockAfterMessage = useMemo(() => {
		const map = new Map<number, TypesGen.ChatUIWorkingBlock>();
		for (const wb of workingBlocks ?? []) {
			map.set(wb.after_message_id, wb);
		}

		// Build a synthetic working block for the current turn when
		// the store has tool-call messages but REST hasn't caught up.
		// This avoids a flash of interior messages between stream
		// clearing and REST refetch.
		if (parsedMessages.length > 0 && lastUserMsgIndex >= 0) {
			const turnMsgs = parsedMessages.slice(lastUserMsgIndex + 1);
			const firstAssistant = turnMsgs[0];
			const hasTools = turnMsgs.some(
				({ parsed }) => parsed.toolCalls.length > 0,
			);
			// Only add synthetic block if the turn has tools AND
			// no REST working block covers it yet.
			if (
				hasTools &&
				firstAssistant &&
				!map.has(firstAssistant.message.id)
			) {
				// The final text message is the last assistant in
				// the turn with no tool calls.
				const lastPureText = [...turnMsgs]
					.reverse()
					.find(
						({ message, parsed }) =>
							message.role === "assistant" &&
							parsed.toolCalls.length === 0 &&
							parsed.markdown.length > 0,
					);
				const interiorCount = turnMsgs.length
					- 1 // first assistant (boundary)
					- (lastPureText ? 1 : 0); // final text (boundary)

				map.set(firstAssistant.message.id, {
					after_message_id: firstAssistant.message.id,
					before_message_id: lastPureText?.message.id ?? 0,
					started_at: firstAssistant.message.created_at,
					ended_at: lastPureText
						? lastPureText.message.created_at
						: undefined,
					message_count: Math.max(interiorCount, 0),
				});
			}
		}

		return map;
	}, [workingBlocks, parsedMessages, lastUserMsgIndex]);

	// Build a set of message IDs that fall inside a working block's
	// range and should be hidden from the timeline. Single O(n)
	// pass — each message checked against the small interval list.
	const hiddenByWorkingBlock = useMemo(() => {
		const hidden = new Set<number>();
		if (workingBlockAfterMessage.size === 0) return hidden;
		const ranges = [...workingBlockAfterMessage.values()].map((wb) => ({
			after: wb.after_message_id,
			before: wb.before_message_id,
		}));
		for (const { message } of parsedMessages) {
			for (const r of ranges) {
				if (message.id > r.after && (r.before === 0 || message.id < r.before)) {
					hidden.add(message.id);
					break;
				}
			}
		}
		return hidden;
	}, [workingBlockAfterMessage, parsedMessages]);

	// Build a set of message IDs that are the final text
	// message after a working block — these animate text in.
	const animateTextMessageIds = useMemo(() => {
		const ids = new Set<number>();
		for (const wb of workingBlockAfterMessage.values()) {
			if (wb.before_message_id > 0) {
				ids.add(wb.before_message_id);
			}
		}
		return ids;
	}, [workingBlockAfterMessage]);

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

	let seenFirstAssistantInCurrentTurn = false;

	return (
		<div className="mx-auto w-full max-w-3xl py-6">
			{isEmpty && !hasStreamOutput ? (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			) : (
				<div className="flex flex-col gap-3">
					{parsedMessages.map(({ message, parsed }, idx) => {
						const isInCurrentTurn = idx > lastUserMsgIndex;

						// During live streaming with tool calls, only the
						// first assistant in the current turn renders (with
						// tool blocks hidden). All other non-user messages
						// are interior — shown inside the live WorkingBlock.
						let isFirstAssistantInCurrentTurn = false;
						if (streamHasToolCalls && isInCurrentTurn && message.role !== "user") {
							if (!seenFirstAssistantInCurrentTurn && message.role === "assistant") {
								seenFirstAssistantInCurrentTurn = true;
								isFirstAssistantInCurrentTurn = true;
							} else {
								return null;
							}
						}

						// Skip messages inside a working block range (REST or synthetic).
						if (hiddenByWorkingBlock.has(message.id)) {
							return null;
						}

						const workingBlock = workingBlockAfterMessage.get(message.id);
						const shouldHideTools = !!workingBlock || isFirstAssistantInCurrentTurn;
						const isActive =
							workingBlock && chatStatus
								? ["running", "pending"].includes(chatStatus)
								: false;

						return (
							<Fragment key={message.id}>
								{message.role === "user" ? (
									<StickyUserMessage
										message={message}
										parsed={parsed}
										onEditUserMessage={onEditUserMessage}
										editingMessageId={editingMessageId}
										savingMessageId={savingMessageId}
										isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
									/>
								) : (
									<ChatMessageItem
										message={message}
										parsed={parsed}
										savingMessageId={savingMessageId}
										urlTransform={urlTransform}
										isAfterEditingMessage={afterEditingMessageIds.has(message.id)}
										hideToolBlocks={shouldHideTools}
										animateText={animateTextMessageIds.has(message.id)}
									/>
								)}
								{workingBlock && !isFirstAssistantInCurrentTurn && (
									<WorkingBlock
										startedAt={workingBlock.started_at}
										endedAt={workingBlock.ended_at}
										isActive={isActive && !workingBlock.ended_at}
										onExpand={async () => {
											// Check if interior messages are already
											// in the store first.
											const storeInterior = parsedMessages
												.filter(({ message: m }) =>
													hiddenByWorkingBlock.has(m.id)
													&& m.id > workingBlock.after_message_id
													&& (workingBlock.before_message_id === 0 || m.id < workingBlock.before_message_id),
												);
											if (storeInterior.length > 0) {
												return storeInterior.map(({ message: m, parsed: p }) => (
													<ChatMessageItem
														key={m.id}
														message={m}
														parsed={p}
														urlTransform={urlTransform}
													/>
												));
											}
											// Fetch from the API for historical turns.
											const resp = await API.experimental.getChatMessages(chatId, {
												before_id: workingBlock.before_message_id || undefined,
												limit: 200,
											});
											// Filter to only messages in this working
											// block's range.
											const rangeMessages = resp.messages
												.filter((m) =>
													m.id > workingBlock.after_message_id
													&& (workingBlock.before_message_id === 0 || m.id < workingBlock.before_message_id),
												)
												.sort((a, b) => a.id - b.id);
											const parsed = parseMessagesWithMergedTools(rangeMessages);
											return parsed.map(({ message: m, parsed: p }) => (
												<ChatMessageItem
													key={m.id}
													message={m}
													parsed={p}
													urlTransform={urlTransform}
												/>
											));
										}}
									/>
								)}
							</Fragment>
						);
					})}
					{shouldRenderStreamAfterMessages && !streamHasToolCalls && (
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
					{shouldRenderStreamAfterMessages && streamHasToolCalls && (
						<WorkingBlock
							startedAt={workingStartRef.current ?? new Date().toISOString()}
							isActive={true}
							defaultExpanded={true}
						>
							{/* Persisted interior messages from completed steps */}
							{(() => {
								// Find the first assistant index so we skip it
								// (it's rendered above the WorkingBlock).
								const firstAsstIdx = parsedMessages.findIndex(
									(e, i) => i > lastUserMsgIndex && e.message.role === "assistant",
								);
								return parsedMessages
									.filter(({ message: m }, idx) =>
										idx > lastUserMsgIndex
										&& m.role !== "user"
										&& idx !== firstAsstIdx,
									)
									.map(({ message: m, parsed: p }) => (
										<ChatMessageItem
											key={m.id}
											message={m}
											parsed={p}
											urlTransform={urlTransform}
										/>
									));
							})()}
							{/* Live stream from current step */}
							<StreamingOutput
								streamState={streamState}
								streamTools={streamTools}
								subagentTitles={subagentTitles}
								subagentStatusOverrides={subagentStatusOverrides}
								urlTransform={urlTransform}
							/>
						</WorkingBlock>
					)}
					{hasStreamOutput && parsedMessages.length === 0 && !streamHasToolCalls && (
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
					{hasStreamOutput && parsedMessages.length === 0 && streamHasToolCalls && (
						<WorkingBlock
							startedAt={workingStartRef.current ?? new Date().toISOString()}
							isActive={true}
						/>
					)}
				</div>
			)}
			{detailError && (
				<Alert
					severity={isUsageLimitError ? "info" : "error"}
					className="py-2"
					actions={
						isUsageLimitError && (
							<Button asChild variant="subtle" size="sm">
								<Link to="/agents/analytics">View Usage</Link>
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
