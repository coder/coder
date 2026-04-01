import type * as TypesGen from "api/typesGenerated";
import { useEffect, useMemo, useRef, useState } from "react";

const DEFAULT_PAGE_SIZE = 50;

type UseMessageWindowOptions = {
	messages: readonly TypesGen.ChatMessage[];
	resetKey?: string;
	pageSize?: number;
};

export const useMessageWindow = ({
	messages,
	resetKey,
	pageSize = DEFAULT_PAGE_SIZE,
}: UseMessageWindowOptions) => {
	const [renderedMessageCount, setRenderedMessageCount] = useState(pageSize);
	const loadMoreSentinelRef = useRef<HTMLDivElement | null>(null);

	useEffect(() => {
		void resetKey;
		setRenderedMessageCount(pageSize);
	}, [resetKey, pageSize]);

	const hasMoreMessages = renderedMessageCount < messages.length;
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
					setRenderedMessageCount((prev) => prev + pageSize);
				}
			},
			{ rootMargin: "200px" },
		);
		observer.observe(node);
		return () => observer.disconnect();
	}, [hasMoreMessages, pageSize]);

	return {
		hasMoreMessages,
		windowedMessages,
		loadMoreSentinelRef,
	};
};
