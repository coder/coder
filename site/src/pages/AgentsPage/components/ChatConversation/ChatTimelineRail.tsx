import {
	type FC,
	type RefObject,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import {
	selectChatStatus,
	selectMessagesByID,
	selectOrderedMessageIDs,
	useChatSelector,
	type useChatStore,
} from "./chatStore";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import type { ParsedMessageEntry } from "./types";

type ChatStoreHandle = ReturnType<typeof useChatStore>["store"];

// PROTOTYPE. Agent conversation timeline rail.
//
// This component renders a minimap-style column of ticks along the right edge
// of the chat scroll container, just to the left of the native scrollbar.
//
// Behaviour, per prototype spec:
// - Rail is hidden by default. It reveals on pointer hover over its column
//   and while the chat is actively scrolling. It fades out ~1.2s after both
//   the pointer leaves and scrolling settles.
// - Each user prompt renders as a grey tick. Assistant messages that contain
//   a failed tool call render as an additional red tick.
// - Hovering a tick shows a floating preview label (first line of the
//   prompt, or a short error label).
// - Clicking a tick smoothly scrolls the anchor into view.
//
// Positions are computed from the DOM: every anchored element carries a
// `data-timeline-anchor-id` attribute placed by `ConversationTimeline`. The
// rail queries them each frame that a re-measure is needed, so it does not
// need to track message layout state directly.

type TimelineItem = {
	id: number;
	role: "user" | "assistant";
	preview: string;
	isError: boolean;
	errorLabel?: string;
};

const HIDE_DELAY_MS = 1200;
const SCROLL_QUIET_MS = 400;
const PREVIEW_MAX_CHARS = 80;

const getTextFromContent = (
	content: readonly TypesGen.ChatMessagePart[] | undefined,
): string => {
	if (!content) {
		return "";
	}
	let text = "";
	for (const part of content) {
		if (part.type === "text") {
			text += part.text;
		}
	}
	return text.replace(/\s+/g, " ").trim();
};

const truncate = (text: string, max: number): string => {
	if (text.length <= max) {
		return text;
	}
	return `${text.slice(0, max - 1).trimEnd()}…`;
};

const buildTimelineItems = (
	parsedMessages: readonly ParsedMessageEntry[],
): TimelineItem[] => {
	const items: TimelineItem[] = [];
	for (const { message, parsed } of parsedMessages) {
		if (message.role === "user") {
			const preview = truncate(
				getTextFromContent(message.content) || "(empty prompt)",
				PREVIEW_MAX_CHARS,
			);
			items.push({
				id: message.id,
				role: "user",
				preview,
				isError: false,
			});
			continue;
		}
		if (message.role === "assistant") {
			const erroredTool = parsed.tools.find((t) => t.isError);
			if (erroredTool) {
				items.push({
					id: message.id,
					role: "assistant",
					preview: `Error in ${erroredTool.name}`,
					isError: true,
					errorLabel: `Error in ${erroredTool.name}`,
				});
			}
		}
	}
	return items;
};

type Position = {
	item: TimelineItem;
};

type MeasureState = {
	positions: Position[];
	activeIndex: number;
};

// Vertical spacing (px) between consecutive ticks. Chosen to match the
// mocked dashes: tight enough to read as a group, loose enough to hit
// individual ticks with a pointer.
const TICK_SPACING_PX = 12;

interface ChatTimelineRailProps {
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	items: readonly TimelineItem[];
}

const ChatTimelineRail: FC<ChatTimelineRailProps> = ({
	scrollContainerRef,
	items,
}) => {
	const railRef = useRef<HTMLDivElement>(null);
	const [state, setState] = useState<MeasureState>({
		positions: [],
		activeIndex: -1,
	});
	const { positions, activeIndex } = state;
	const [visible, setVisible] = useState(false);
	const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
	const hoverRef = useRef(false);
	const scrollingRef = useRef(false);
	const hideTimerRef = useRef<number | null>(null);
	const scrollQuietTimerRef = useRef<number | null>(null);

	const itemsById = useMemo(() => {
		const map = new Map<number, TimelineItem>();
		for (const item of items) {
			map.set(item.id, item);
		}
		return map;
	}, [items]);

	// Recomputes tick positions from DOM anchor order. Called on scroll,
	// resize, and DOM mutation. Uses rAF batching to avoid layout thrash.
	//
	// Ticks are laid out with fixed pixel spacing, centered vertically on
	// the rail. This keeps the column feeling like a compact indicator
	// stack rather than a justified min-to-max scale, which felt clunky
	// for chats with only a few prompts.
	const measure = useCallback(() => {
		const scroller = scrollContainerRef.current;
		if (!scroller) {
			setState({ positions: [], activeIndex: -1 });
			return;
		}
		const anchors = scroller.querySelectorAll<HTMLElement>(
			"[data-timeline-anchor-id]",
		);
		if (anchors.length === 0) {
			setState({ positions: [], activeIndex: -1 });
			return;
		}
		const scrollerTop = scroller.getBoundingClientRect().top;
		// The active tick is the last anchor whose top has scrolled to or
		// above the top edge of the viewport, with a small offset so it
		// flips as the message crosses that threshold rather than only
		// once it fully leaves.
		const ACTIVE_OFFSET = 24;
		const ordered: Position[] = [];
		let activeIdx = -1;
		for (const anchor of anchors) {
			const rawId = anchor.getAttribute("data-timeline-anchor-id");
			if (!rawId) continue;
			const id = Number(rawId);
			const item = itemsById.get(id);
			if (!item) continue;
			const rect = anchor.getBoundingClientRect();
			const pastTop = rect.top - scrollerTop <= ACTIVE_OFFSET;
			if (pastTop) {
				activeIdx = ordered.length;
			}
			ordered.push({ item });
		}
		if (ordered.length === 0) {
			setState({ positions: [], activeIndex: -1 });
			return;
		}
		// If nothing has scrolled past the threshold yet, treat the first
		// prompt as active so the indicator is always shown.
		if (activeIdx === -1) {
			activeIdx = 0;
		}
		setState({ positions: ordered, activeIndex: activeIdx });
	}, [scrollContainerRef, itemsById]);

	useLayoutEffect(() => {
		measure();
	}, [measure]);

	useEffect(() => {
		const scroller = scrollContainerRef.current;
		if (!scroller) return;
		let raf = 0;
		const schedule = () => {
			if (raf) return;
			raf = requestAnimationFrame(() => {
				raf = 0;
				measure();
			});
		};
		const observer = new ResizeObserver(schedule);
		observer.observe(scroller);
		const mo = new MutationObserver(schedule);
		mo.observe(scroller, {
			childList: true,
			subtree: true,
			attributes: true,
			attributeFilter: [
				"data-timeline-anchor-id",
				"data-timeline-anchor-error",
			],
		});
		return () => {
			observer.disconnect();
			mo.disconnect();
			if (raf) cancelAnimationFrame(raf);
		};
	}, [scrollContainerRef, measure]);

	// Compute visibility from pointer hover + scroll activity. Kept in a
	// dedicated helper so both signals can flip the visible state through
	// the same debounce.
	const updateVisibility = useCallback(() => {
		const shouldShow = hoverRef.current || scrollingRef.current;
		if (shouldShow) {
			if (hideTimerRef.current) {
				window.clearTimeout(hideTimerRef.current);
				hideTimerRef.current = null;
			}
			setVisible(true);
			return;
		}
		if (hideTimerRef.current) return;
		hideTimerRef.current = window.setTimeout(() => {
			hideTimerRef.current = null;
			setVisible(false);
			setHoveredIndex(null);
		}, HIDE_DELAY_MS);
	}, []);

	useEffect(() => {
		const scroller = scrollContainerRef.current;
		if (!scroller) return;
		let raf = 0;
		const onScroll = () => {
			scrollingRef.current = true;
			updateVisibility();
			if (scrollQuietTimerRef.current) {
				window.clearTimeout(scrollQuietTimerRef.current);
			}
			scrollQuietTimerRef.current = window.setTimeout(() => {
				scrollingRef.current = false;
				updateVisibility();
			}, SCROLL_QUIET_MS);
			if (raf) return;
			raf = requestAnimationFrame(() => {
				raf = 0;
				measure();
			});
		};
		scroller.addEventListener("scroll", onScroll, { passive: true });
		return () => {
			scroller.removeEventListener("scroll", onScroll);
			if (raf) cancelAnimationFrame(raf);
			if (scrollQuietTimerRef.current) {
				window.clearTimeout(scrollQuietTimerRef.current);
			}
		};
	}, [scrollContainerRef, measure, updateVisibility]);

	useEffect(() => {
		return () => {
			if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
			if (scrollQuietTimerRef.current)
				window.clearTimeout(scrollQuietTimerRef.current);
		};
	}, []);

	const handleJump = useCallback(
		(id: number) => {
			const scroller = scrollContainerRef.current;
			if (!scroller) return;
			const anchor = scroller.querySelector<HTMLElement>(
				`[data-timeline-anchor-id="${CSS.escape(String(id))}"]`,
			);
			if (!anchor) return;
			anchor.scrollIntoView({ behavior: "smooth", block: "start" });
		},
		[scrollContainerRef],
	);

	const handleEnter = () => {
		hoverRef.current = true;
		updateVisibility();
	};
	const handleLeave = () => {
		hoverRef.current = false;
		updateVisibility();
	};

	if (items.length === 0) {
		return null;
	}

	return (
		<div
			ref={railRef}
			data-testid="chat-timeline-rail"
			aria-hidden
			className={cn(
				"pointer-events-auto absolute inset-y-2 right-1 z-20 w-6",
				"transition-opacity duration-200",
				visible ? "opacity-100" : "opacity-0",
			)}
			onMouseEnter={handleEnter}
			onMouseLeave={handleLeave}
			onFocus={handleEnter}
			onBlur={handleLeave}
		>
			{positions.map((pos, idx) => {
				const isHovered = hoveredIndex === idx;
				const isActive = activeIndex === idx;
				// Center the whole stack vertically on the rail, then offset
				// each tick by its index. calc(50% + Npx) keeps the group
				// centered regardless of the rail's actual height.
				const offsetPx = (idx - (positions.length - 1) / 2) * TICK_SPACING_PX;
				return (
					<button
						type="button"
						key={pos.item.id}
						onMouseEnter={() => setHoveredIndex(idx)}
						onMouseLeave={() =>
							setHoveredIndex((current) => (current === idx ? null : current))
						}
						onFocus={() => setHoveredIndex(idx)}
						onBlur={() =>
							setHoveredIndex((current) => (current === idx ? null : current))
						}
						onClick={() => handleJump(pos.item.id)}
						className={cn(
							"absolute inset-x-0 -translate-y-1/2",
							"flex h-3 items-center justify-center border-0 bg-transparent p-0",
						)}
						style={{ top: `calc(50% + ${offsetPx}px)` }}
						aria-label={pos.item.preview}
					>
						<span
							aria-hidden
							className={cn(
								"block rounded-full transition-[width,height,background-color] duration-150",
								pos.item.isError
									? "bg-content-destructive/70"
									: "bg-content-secondary/50",
								isHovered
									? cn(
											"h-[3px] w-5",
											pos.item.isError
												? "bg-content-destructive"
												: "bg-content-primary",
										)
									: isActive
										? cn(
												"h-[3px] w-4",
												pos.item.isError
													? "bg-content-destructive"
													: "bg-content-primary",
											)
										: "h-[2px] w-3",
							)}
						/>
						{isHovered && (
							<span
								className={cn(
									"pointer-events-none absolute right-full top-1/2 mr-2 -translate-y-1/2",
									"whitespace-nowrap rounded-md border px-2 py-1 text-xs shadow-md",
									"bg-surface-primary text-content-primary border-border-default",
									pos.item.isError &&
										"text-content-destructive border-border-destructive",
								)}
							>
								{pos.item.preview}
							</span>
						)}
					</button>
				);
			})}
		</div>
	);
};

// Store-connected wrapper. Kept in the same module so consumers only need
// to thread the scroll container ref and the chat store handle.
interface ChatTimelineRailContainerProps {
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	store: ChatStoreHandle;
}

const isChatMessage = (
	m: TypesGen.ChatMessage | undefined,
): m is TypesGen.ChatMessage => Boolean(m);

export const ChatTimelineRailContainer: FC<ChatTimelineRailContainerProps> = ({
	scrollContainerRef,
	store,
}) => {
	const messagesByID = useChatSelector(store, selectMessagesByID);
	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const chatStatus = useChatSelector(store, selectChatStatus);
	const items = useMemo(() => {
		const messages = orderedMessageIDs
			.map((id) => messagesByID.get(id))
			.filter(isChatMessage);
		const pendingToolCallIDs = getPendingToolCallIDs(messages, chatStatus);
		const parsed = parseMessagesWithMergedTools(messages, {
			pendingToolCallIDs,
		});
		return buildTimelineItems(parsed);
	}, [orderedMessageIDs, messagesByID, chatStatus]);
	return (
		<ChatTimelineRail scrollContainerRef={scrollContainerRef} items={items} />
	);
};
