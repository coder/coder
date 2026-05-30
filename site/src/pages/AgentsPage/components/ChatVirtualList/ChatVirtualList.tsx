import {
	type FC,
	type ReactNode,
	type RefObject,
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import {
	createHeightCache,
	type HeightCache,
	type MessageKind,
} from "./heightCache";
import { ScrollToBottomButton } from "./ScrollToBottomButton";
import { useScrollAnchor } from "./useScrollAnchor";
import { computeWindow, cumulativeOffsets } from "./windowMath";

export type VirtualItem = { id: string; kind: MessageKind };

// Render this many pixels above and below the viewport, matching the
// @pierre/diffs Virtualizer overscan so fast scrolls do not blank.
const VIRTUAL_OVERSCAN = 1000;
const LOAD_MORE_MARGIN = "600px 0px 0px 0px";

type ChatVirtualListProps = {
	items: ReadonlyArray<VirtualItem>;
	renderItem: (item: VirtualItem) => ReactNode;
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
};

// The cached height sizes this item's spacer once it scrolls out of the
// window, so leaving the window is layout-neutral.
const MeasuredItem: FC<{
	item: VirtualItem;
	renderItem: (item: VirtualItem) => ReactNode;
	cache: HeightCache;
	onMeasured: () => void;
}> = ({ item, renderItem, cache, onMeasured }) => {
	const ref = useRef<HTMLDivElement | null>(null);
	useEffect(() => {
		const el = ref.current;
		if (!el) {
			return;
		}
		const observer = new ResizeObserver(() => {
			const height = el.getBoundingClientRect().height;
			if (height !== cache.get(item.id)) {
				cache.record(item.id, item.kind, height);
				onMeasured();
			}
		});
		observer.observe(el);
		return () => observer.disconnect();
	}, [item, cache, onMeasured]);

	return (
		<div ref={ref} data-chat-item="" data-chat-item-id={item.id}>
			{renderItem(item)}
		</div>
	);
};

export const ChatVirtualList: FC<ChatVirtualListProps> = ({
	items,
	renderItem,
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
}) => {
	const {
		scrollerRef,
		contentRef,
		atBottom,
		scrollToBottom,
		captureAnchor,
		restoreAnchor,
	} = useScrollAnchor();
	const topSentinelRef = useRef<HTMLDivElement | null>(null);
	// Lazy useState initializer keeps one cache instance for the component's life
	// without reading a ref during render, which the React Compiler forbids.
	const [cache] = useState<HeightCache>(() => createHeightCache());

	const [scrollTop, setScrollTop] = useState(0);
	const [viewportHeight, setViewportHeight] = useState(0);
	const [cacheVersion, setCacheVersion] = useState(0);
	const bumpCacheVersion = useCallback(() => {
		setCacheVersion((value) => value + 1);
	}, []);

	const setScroller = useCallback(
		(element: HTMLDivElement | null) => {
			scrollerRef.current = element;
			scrollContainerRef.current = element;
			scrollToBottomRef.current = element ? scrollToBottom : null;
		},
		[scrollerRef, scrollContainerRef, scrollToBottomRef, scrollToBottom],
	);

	const heights = items.map((item) => cache.estimate(item.id, item.kind));
	const offsets = cumulativeOffsets(heights);
	const { start, end, topPad, bottomPad } = computeWindow({
		offsets,
		scrollTop,
		// Until the scroller is measured, fall back to a 1px viewport so the first
		// window stays small (overscan only) instead of mounting the whole list.
		viewportHeight: viewportHeight || 1,
		overscan: VIRTUAL_OVERSCAN,
	});
	const visible = end >= start ? items.slice(start, end + 1) : [];

	// Mirror the live scroll position into state so the window recomputes as the
	// reader scrolls. The anchor is captured by the layout effect below, after the
	// new window commits, never here. A teleport scroll therefore records a real
	// rendered item instead of a stale one from the previous window.
	useEffect(() => {
		const scroller = scrollerRef.current;
		if (!scroller) {
			return;
		}
		let frame = 0;
		const onScroll = () => {
			if (frame) {
				return;
			}
			frame = requestAnimationFrame(() => {
				frame = 0;
				setScrollTop(scroller.scrollTop);
			});
		};
		scroller.addEventListener("scroll", onScroll, { passive: true });
		return () => {
			scroller.removeEventListener("scroll", onScroll);
			if (frame) {
				cancelAnimationFrame(frame);
			}
		};
	}, [scrollerRef]);

	// A resize counts as a content change below, so the anchor (or bottom pin)
	// is preserved across it.
	useEffect(() => {
		const scroller = scrollerRef.current;
		if (!scroller) {
			return;
		}
		const observer = new ResizeObserver(() => {
			setViewportHeight(scroller.clientHeight);
		});
		observer.observe(scroller);
		setViewportHeight(scroller.clientHeight);
		return () => observer.disconnect();
	}, [scrollerRef]);

	// Single scrollTop owner. After every commit, classify the change by diffing
	// the committed inputs against the previous commit:
	//   - content changed (items, measured heights, or viewport): the DOM shifted
	//     under the reader, so restore the captured anchor (or re-pin to the
	//     bottom), then re-capture at the settled position.
	//   - only scrollTop changed (a deliberate scroll): just re-capture; restoring
	//     here would fight the scroll.
	// restore writes scroller.scrollTop, which the scroll listener turns into the
	// next scrollTop state, so a correction converges across frames.
	const prevCommitRef = useRef<{
		scrollTop: number;
		items: ReadonlyArray<VirtualItem>;
		cacheVersion: number;
		viewportHeight: number;
	} | null>(null);
	useLayoutEffect(() => {
		const prev = prevCommitRef.current;
		prevCommitRef.current = { scrollTop, items, cacheVersion, viewportHeight };
		if (!prev) {
			restoreAnchor();
			captureAnchor();
			return;
		}
		const contentChanged =
			items !== prev.items ||
			cacheVersion !== prev.cacheVersion ||
			viewportHeight !== prev.viewportHeight;
		if (contentChanged) {
			restoreAnchor();
			captureAnchor();
		} else if (scrollTop !== prev.scrollTop) {
			captureAnchor();
		}
	}, [
		scrollTop,
		viewportHeight,
		items,
		cacheVersion,
		restoreAnchor,
		captureAnchor,
	]);

	useEffect(() => {
		const sentinel = topSentinelRef.current;
		const scroller = scrollerRef.current;
		if (!sentinel || !scroller || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			([entry]) => {
				if (
					entry.isIntersecting &&
					hasMoreMessages &&
					!isFetchingMoreMessages
				) {
					onFetchMoreMessages();
				}
			},
			{ root: scroller, rootMargin: LOAD_MORE_MARGIN },
		);
		observer.observe(sentinel);
		return () => observer.disconnect();
	}, [
		scrollerRef,
		hasMoreMessages,
		isFetchingMoreMessages,
		onFetchMoreMessages,
	]);

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={setScroller}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [scrollbar-gutter:stable]"
			>
				<div ref={contentRef} className="flex flex-col">
					<div ref={topSentinelRef} aria-hidden className="h-0" />
					<div data-virtual-spacer="" style={{ height: topPad }} />
					{visible.map((item) => (
						<MeasuredItem
							key={item.id}
							item={item}
							renderItem={renderItem}
							cache={cache}
							onMeasured={bumpCacheVersion}
						/>
					))}
					<div data-virtual-spacer="" style={{ height: bottomPad }} />
				</div>
			</div>
			<ScrollToBottomButton
				visible={!atBottom}
				onScrollToBottom={scrollToBottom}
			/>
		</div>
	);
};
