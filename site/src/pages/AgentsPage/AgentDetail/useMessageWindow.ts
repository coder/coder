import type * as TypesGen from "api/typesGenerated";
import { useEffect, useMemo, useRef, useState } from "react";

const DEFAULT_PAGE_SIZE = 50;

type UseMessageWindowOptions = {
	messages: readonly TypesGen.ChatMessage[];
	resetKey?: string;
	pageSize?: number;
	hasMoreOnServer?: boolean;
	onFetchMore?: () => void;
	isFetchingMore?: boolean;
};

export const useMessageWindow = ({
	messages,
	resetKey,
	pageSize = DEFAULT_PAGE_SIZE,
	hasMoreOnServer = false,
	onFetchMore,
	isFetchingMore = false,
}: UseMessageWindowOptions) => {
	const [renderedMessageCount, setRenderedMessageCount] = useState(pageSize);
	const loadMoreSentinelRef = useRef<HTMLDivElement | null>(null);

	useEffect(() => {
		void resetKey;
		setRenderedMessageCount(pageSize);
	}, [resetKey, pageSize]);

	const hasMoreLocal = renderedMessageCount < messages.length;
	const hasMoreMessages = hasMoreLocal || hasMoreOnServer;

	const windowedMessages = useMemo(() => {
		if (renderedMessageCount >= messages.length) {
			return messages;
		}
		return messages.slice(messages.length - renderedMessageCount);
	}, [messages, renderedMessageCount]);

	useEffect(() => {
		const node = loadMoreSentinelRef.current;
		if (!node || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					if (hasMoreLocal) {
						// Still have local messages to show.
						setRenderedMessageCount((prev) => prev + pageSize);
					} else if (hasMoreOnServer && onFetchMore && !isFetchingMore) {
						// Exhausted local messages, fetch from server.
						onFetchMore();
					}
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(node);
		return () => observer.disconnect();
	}, [
		hasMoreMessages,
		hasMoreLocal,
		hasMoreOnServer,
		onFetchMore,
		isFetchingMore,
		pageSize,
	]);

	return {
		hasMoreMessages,
		windowedMessages,
		loadMoreSentinelRef,
		isFetchingMore,
	};
};
