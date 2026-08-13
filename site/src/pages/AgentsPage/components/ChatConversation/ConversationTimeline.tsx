import {
	ChevronLeftIcon,
	ChevronRightIcon,
	InfoIcon,
	PencilIcon,
} from "lucide-react";
import {
	type FC,
	memo,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";

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
	// Durable rows render from a message. The live assistant row renders from
	// liveStatus and the stream buffers instead.
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
				// User rows render inside StickyUserMessage, which owns the row
				// identity attributes for both the flow copy and the sticky copy.
				data-testid={isUser ? undefined : `chat-message-${renderKey}`}
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
							fadeFromBottom={fadeFromBottom}
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
		const [isStuck, setIsStuck] = useState(false);
		const [isReady, setIsReady] = useState(false);
		const [isTooTall, setIsTooTall] = useState(false);
		const sentinelRef = useRef<HTMLDivElement>(null);
		const messageKey = `message:${message.id}`;
		const messageId = message.id;
		const setSentinelRef = (el: HTMLDivElement | null) => {
			sentinelRef.current = el;
			registerSentinel?.(messageId, el);
		};
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

			const update = () => {
				// Read the scroller geometry on each tick. Caching it goes
				// stale when the scroller moves or resizes without a window
				// resize (for example the composer growing), which skews the
				// clip height and push-up math.
				const scrollerTop = scroller.getBoundingClientRect().top;
				const scrollerHeight = scroller.clientHeight;
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

			// Re-run the visual update when the transcript height changes,
			// for example a streaming response or several messages arriving
			// at once. In flex-col-reverse the scrollTop stays at 0 while
			// pinned to the bottom, so no scroll event fires; observing the
			// content wrapper catches that growth instead.
			//
			// The scroller's firstElementChild is the flex spacer that pins
			// content to the bottom. It collapses to 0px once the transcript
			// overflows and then stops emitting resize callbacks, which is
			// exactly when truncation is active, so observe the real content
			// node (an ancestor of the sentinel) and fall back to the spacer
			// only when the marker is absent.
			const contentEl =
				sentinel.closest<HTMLElement>("[data-chat-scroll-content]") ??
				(scroller.firstElementChild as HTMLElement | null);
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
			window.addEventListener("resize", update);
			update();
			// Set immediately — both --clip-h and --overlay-ready are
			// applied before the browser paints since we're in a
			// useLayoutEffect.
			container.style.setProperty("--overlay-ready", "1");
			return () => {
				scroller.removeEventListener("scroll", onScroll);
				window.removeEventListener("resize", update);
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
				<div ref={setSentinelRef} className="h-0" data-user-sentinel />
				<div
					ref={containerRef}
					data-testid={`chat-message-${messageKey}`}
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
						// While the overlay copy is shown, drop the flow copy
						// from the accessibility tree so the message and its
						// hook notices aren't exposed twice.
						aria-hidden={isStuck && !isTooTall ? true : undefined}
						inert={isStuck && !isTooTall ? true : undefined}
					>
						<ChatMessageItem
							renderKey={messageKey}
							message={message}
							parsed={parsed}
							onEditUserMessage={handleEditUserMessage}
							editingMessageId={editingMessageId}
							isAfterEditingMessage={isAfterEditingMessage}
							prevUserMessageId={prevUserMessageId}
							nextUserMessageId={nextUserMessageId}
							onJumpToUserMessage={onJumpToUserMessage}
							urlTransform={urlTransform}
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
									renderKey={messageKey}
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
						</div>
					)}
				</div>
			</>
		);
	},
);

interface ConversationTimelineProps {
	parsedMessages: readonly ParsedMessageEntry[];
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
		const renderRows = assignTimelineRows(
			displayMessages,
			Boolean(liveStatus && shouldRenderLiveAssistant(liveStatus)),
		);

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

		// Ordered list of visible user message IDs, used to drive the
		// per-bubble prev/next arrow buttons that jump the transcript
		// to the neighbouring user prompt.
		const visibleUserMessageIds: number[] = [];
		for (const { message } of displayMessages) {
			if (message.role === "user") {
				visibleUserMessageIds.push(message.id);
			}
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
				<div
					data-testid="conversation-timeline"
					className="flex flex-col gap-2"
				>
					{renderRows.map((row) => {
						if (row.type === "live") {
							// This row only exists when liveStatus is set.
							return (
								<ChatMessageItem
									key={row.key}
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
							);
						}
						const { message, parsed } = row.entry;
						const neighbors = userNeighborsById.get(message.id);
						const isAfterEditingMessage = afterEditingMessageIds.has(
							message.id,
						);
						if (message.role === "user") {
							return (
								<StickyUserMessage
									key={row.key}
									message={message}
									parsed={parsed}
									onEditUserMessage={onEditUserMessage}
									editingMessageId={editingMessageId}
									isAfterEditingMessage={isAfterEditingMessage}
									prevUserMessageId={neighbors?.prevId}
									nextUserMessageId={neighbors?.nextId}
									onJumpToUserMessage={jumpToUserMessage}
									registerSentinel={registerSentinel}
									urlTransform={urlTransform}
								/>
							);
						}
						return (
							<ChatMessageItem
								key={row.key}
								renderKey={row.key}
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
								isAfterEditingMessage={isAfterEditingMessage}
								hideActions={!row.isLastInAssistantChain}
								hasActiveStream={Boolean(hasActiveStream)}
								isAwaitingFirstStreamChunk={Boolean(isAwaitingFirstStreamChunk)}
								isLastMessage={row.isLastMessage}
								mcpServers={mcpServers}
								subagentTitles={subagentTitles}
								subagentVariants={subagentVariants}
								showDesktopPreviews={showDesktopPreviews}
							/>
						);
					})}
				</div>
			</FileProbeProvider>
		);
	},
);
