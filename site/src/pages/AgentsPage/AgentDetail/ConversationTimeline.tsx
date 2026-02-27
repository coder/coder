import type * as TypesGen from "api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	Response,
	Shimmer,
	Tool,
} from "components/ai-elements";
import { ChevronDownIcon, Loader2Icon } from "lucide-react";
import {
	type FC,
	memo,
	type ReactNode,
	type RefObject,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import type {
	MergedTool,
	ParsedMessageContent,
	ParsedMessageSection,
	RenderBlock,
	StreamState,
} from "./types";

const ReasoningDisclosure: FC<{
	id: string;
	title?: string;
	text: string;
	isStreaming?: boolean;
}> = ({ id, title, text, isStreaming = false }) => {
	const [isOpen, setIsOpen] = useState(false);
	const hasText = text.trim().length > 0;
	const label = title ?? "Thinking";
	const showStreamingPlaceholder = isStreaming && !hasText;

	if (!title && hasText) {
		return (
			<div className="w-full">
				<Response className="text-[11px] text-content-secondary">
					{text}
				</Response>
			</div>
		);
	}

	const labelContent = (
		<span className="text-sm">
			{showStreamingPlaceholder ? (
				<Shimmer as="span">Thinking...</Shimmer>
			) : (
				label
			)}
		</span>
	);

	return (
		<div className="w-full">
			{hasText ? (
				<div
					role="button"
					tabIndex={0}
					aria-expanded={isOpen}
					aria-controls={id}
					className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary cursor-pointer"
					onClick={() => setIsOpen((prev) => !prev)}
					onKeyDown={(event) => {
						if (event.key === "Enter" || event.key === " ") {
							setIsOpen((prev) => !prev);
						}
					}}
				>
					{labelContent}
					<ChevronDownIcon
						className={cn(
							"h-3 w-3 shrink-0 text-content-secondary transition-transform",
							isOpen ? "rotate-0" : "-rotate-90",
						)}
					/>
				</div>
			) : (
				<div className="flex items-center gap-2 text-content-secondary transition-colors hover:text-content-primary">
					{labelContent}
				</div>
			)}
			{isOpen && hasText ? (
				<div id={id} className="mt-1.5">
					<Response className="text-[11px] text-content-secondary">
						{text}
					</Response>
				</div>
			) : null}
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
}: RenderBlockListParams): RenderBlockListResult {
	const renderedToolIDs = new Set<string>();
	const elements = blocks
		.map((block, index) => {
			switch (block.type) {
				case "response":
					return (
						<Response key={`${keyPrefix}-response-${index}`}>
							{block.text}
						</Response>
					);
				case "thinking":
					return (
						<ReasoningDisclosure
							key={`${keyPrefix}-thinking-${index}`}
							id={`${keyPrefix}-thinking-${index}`}
							title={block.title}
							text={block.text}
							isStreaming={isStreaming}
						/>
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
	onEditUserMessage?: (messageId: number, text: string) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
	// When true, renders a gradient overlay inside the bubble
	// that fades text out toward the bottom. Used by the sticky
	// overlay to indicate truncated content.
	fadeFromBottom?: boolean;
}>(
	({
		message,
		parsed,
		onEditUserMessage,
		editingMessageId,
		savingMessageId,
		fadeFromBottom = false,
	}) => {
		const isUser = message.role === "user";
		const isSavingMessage = savingMessageId === message.id;
		const toolByID = new Map(parsed.tools.map((tool) => [tool.id, tool]));

		if (
			parsed.toolResults.length > 0 &&
			parsed.toolCalls.length === 0 &&
			parsed.markdown === "" &&
			parsed.reasoning === ""
		) {
			return null;
		}

		const hasRenderableContent =
			parsed.blocks.length > 0 || parsed.tools.length > 0;
		const conversationItemProps: { role: "user" | "assistant" } = {
			role: isUser ? "user" : "assistant",
		};
		const { elements: orderedBlocks, renderedToolIDs } = renderBlockList({
			blocks: parsed.blocks,
			toolByID,
			keyPrefix: String(message.id),
		});
		const remainingTools = parsed.tools.filter(
			(tool) => !renderedToolIDs.has(tool.id),
		);

		return (
			<ConversationItem {...conversationItemProps}>
				{isUser ? (
					<Message className="my-2 w-full max-w-none">
						<MessageContent
							className={cn(
								"rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm transition-all",
								onEditUserMessage &&
									!isSavingMessage &&
									"cursor-pointer hover:bg-surface-tertiary",
								editingMessageId === message.id &&
									"ring-2 ring-content-link/40",
								isSavingMessage && "ring-2 ring-content-secondary/40",
								fadeFromBottom && "relative overflow-hidden",
							)}
							style={
								fadeFromBottom
									? { maxHeight: "var(--clip-h, none)" }
									: undefined
							}
							onClick={
								onEditUserMessage && !isSavingMessage
									? () => onEditUserMessage(message.id, parsed.markdown || "")
									: undefined
							}
						>
							<div className="flex items-start gap-2">
								<span className="min-w-0 flex-1">{parsed.markdown || ""}</span>
								{isSavingMessage && (
									<Loader2Icon
										className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-spin text-content-secondary"
										aria-label="Saving message edit"
									/>
								)}
							</div>
							{fadeFromBottom && (
								<div
									className="pointer-events-none absolute inset-x-0 bottom-0 h-12"
									style={{
										background:
											"linear-gradient(to top, hsl(var(--surface-secondary)), transparent)",
									}}
								/>
							)}
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
		);
	},
);
ChatMessageItem.displayName = "ChatMessageItem";

const StreamingOutput = memo<{
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles?: Map<string, string>;
	subagentStatusOverrides?: Map<string, TypesGen.ChatStatus>;
	showInitialPlaceholder?: boolean;
}>(
	({
		streamState,
		streamTools,
		subagentTitles,
		subagentStatusOverrides,
		showInitialPlaceholder = false,
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
										Thinking...
									</Response>
									<div className="pointer-events-none absolute inset-0">
										<Shimmer as="div" className="text-[13px] leading-relaxed">
											Thinking...
										</Shimmer>
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
	onEditUserMessage?: (messageId: number, text: string) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
}> = ({
	message,
	parsed,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
}) => {
	const [isStuck, setIsStuck] = useState(false);
	const [isReady, setIsReady] = useState(false);
	const sentinelRef = useRef<HTMLDivElement>(null);
	const containerRef = useRef<HTMLDivElement>(null);

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

		const update = () => {
			const fullHeight = container.offsetHeight;
			const sentinelTop = sentinel.getBoundingClientRect().top;
			const scrolledPast = scrollerTop - sentinelTop;

			if (scrolledPast <= 0) {
				// Always set a valid value so the overlay has the
				// correct height immediately when isStuck flips.
				container.style.setProperty("--clip-h", `${fullHeight}px`);
				container.style.setProperty("--fade-opacity", "0");
				return;
			}

			const visible = Math.max(fullHeight - scrolledPast, MIN_HEIGHT);
			container.style.setProperty("--clip-h", `${visible}px`);
			// Only show the fade gradient once enough content is
			// clipped to be visually meaningful.
			container.style.setProperty(
				"--fade-opacity",
				visible < fullHeight - 8 ? "1" : "0",
			);
		};

		const onResize = () => {
			scrollerTop = scroller.getBoundingClientRect().top;
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

	const handleEditUserMessage = onEditUserMessage
		? (messageId: number, text: string) => {
				onEditUserMessage(messageId, text);
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
			<div ref={sentinelRef} className="h-0" />
			<div
				ref={containerRef}
				className={cn(
					"relative sticky top-0 z-10 px-3 -mx-3 pt-2 pb-2",
					!isReady && "invisible",
					isStuck && "pointer-events-none",
				)}
			>
				{/* Flow element: always in the DOM to preserve
				    scroll layout. Hidden when stuck so the
				    clipped overlay takes over visually. */}
				<div
					className={isStuck ? undefined : "pointer-events-auto"}
					style={
						isStuck
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
					/>
				</div>

				{/* Overlay: absolutely positioned, matching the
				    sticky container. max-height + mask are driven
				    entirely by the --clip-h CSS variable which the
				    scroll handler sets on the container. */}
				{isStuck && (
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
						{/* Content layer: px-3 pt-2 matches the
						    sticky container's padding so the
						    overlay aligns with the flow element.
						    will-change promotes to GPU layer. */}
						<div
							className="relative px-3 pt-2 pointer-events-auto"
							style={{ willChange: "max-height" }}
						>
							<ChatMessageItem
								message={message}
								parsed={parsed}
								onEditUserMessage={handleEditUserMessage}
								editingMessageId={editingMessageId}
								savingMessageId={savingMessageId}
								fadeFromBottom
							/>
						</div>
					</div>
				)}
			</div>
		</>
	);
};

type ConversationTimelineProps = {
	isEmpty: boolean;
	hasMoreMessages: boolean;
	loadMoreSentinelRef: RefObject<HTMLDivElement | null>;
	parsedSections: readonly ParsedMessageSection[];
	hasStreamOutput: boolean;
	streamState: StreamState | null;
	streamTools: readonly MergedTool[];
	subagentTitles: Map<string, string>;
	subagentStatusOverrides: Map<string, TypesGen.ChatStatus>;
	isAwaitingFirstStreamChunk: boolean;
	detailErrorMessage?: string | null;
	onEditUserMessage?: (messageId: number, text: string) => void;
	editingMessageId?: number | null;
	savingMessageId?: number | null;
};

export const ConversationTimeline: FC<ConversationTimelineProps> = ({
	isEmpty,
	hasMoreMessages,
	loadMoreSentinelRef,
	parsedSections,
	hasStreamOutput,
	streamState,
	streamTools,
	subagentTitles,
	subagentStatusOverrides,
	isAwaitingFirstStreamChunk,
	detailErrorMessage,
	onEditUserMessage,
	editingMessageId,
	savingMessageId,
}) => {
	const shouldRenderStreamInLastSection =
		hasStreamOutput && parsedSections.length > 0;

	return (
		<div className="mx-auto w-full max-w-3xl py-6">
			{isEmpty && !hasStreamOutput ? (
				<div className="py-12 text-center text-content-secondary">
					<p className="text-sm">Start a conversation with your agent.</p>
				</div>
			) : (
				<div className="flex flex-col">
					{hasMoreMessages && (
						<div
							ref={loadMoreSentinelRef}
							className="flex items-center justify-center py-4 text-xs text-content-secondary"
						>
							Loading earlier messages…
						</div>
					)}
					{parsedSections.map((section, sectionIdx) => (
						<div
							key={section.userEntry?.message.id ?? `section-${sectionIdx}`}
							className="-mx-1 px-1"
							style={{
								contentVisibility: "auto",
								containIntrinsicSize: "1px 600px",
							}}
						>
							<div className="flex flex-col gap-3">
								{section.entries.map(({ message, parsed }) =>
									message.role === "user" ? (
										<StickyUserMessage
											key={message.id}
											message={message}
											parsed={parsed}
											onEditUserMessage={onEditUserMessage}
											editingMessageId={editingMessageId}
											savingMessageId={savingMessageId}
										/>
									) : (
										<ChatMessageItem
											key={message.id}
											message={message}
											parsed={parsed}
											savingMessageId={savingMessageId}
										/>
									),
								)}
								{shouldRenderStreamInLastSection &&
									sectionIdx === parsedSections.length - 1 && (
										<StreamingOutput
											streamState={streamState}
											streamTools={streamTools}
											subagentTitles={subagentTitles}
											subagentStatusOverrides={subagentStatusOverrides}
											showInitialPlaceholder={isAwaitingFirstStreamChunk}
										/>
									)}
							</div>
						</div>
					))}
					{hasStreamOutput && parsedSections.length === 0 && (
						<StreamingOutput
							streamState={streamState}
							streamTools={streamTools}
							subagentTitles={subagentTitles}
							subagentStatusOverrides={subagentStatusOverrides}
							showInitialPlaceholder={isAwaitingFirstStreamChunk}
						/>
					)}
				</div>
			)}
			{detailErrorMessage && (
				<div className="mt-4 rounded-md border border-border-destructive bg-surface-red px-3 py-2 text-xs text-content-destructive">
					{detailErrorMessage}
				</div>
			)}
		</div>
	);
};
