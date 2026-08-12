import { type FC, useEffect, useEffectEvent, useRef } from "react";
import { Spinner } from "#/components/Spinner/Spinner";

export const LoadMoreSentinel: FC<{
	onLoadMore?: () => void;
	isFetchingNextPage?: boolean;
}> = ({ onLoadMore, isFetchingNextPage }) => {
	const sentinelRef = useRef<HTMLDivElement>(null);
	const onLoadMoreEvent = useEffectEvent(() => {
		onLoadMore?.();
	});

	useEffect(() => {
		// Don't observe while a fetch is in progress. When the
		// fetch completes this effect re-runs, creating a fresh
		// observer whose initial entry detects the sentinel if
		// it's still visible, fixing the case where loaded items
		// don't push the sentinel out of view and the previous
		// observer never re-fires.
		if (isFetchingNextPage) return;

		const el = sentinelRef.current;
		if (!el) return;

		const observer = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) {
					onLoadMoreEvent();
				}
			},
			{ threshold: 0 },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, [isFetchingNextPage]);

	return (
		<div ref={sentinelRef} className="flex items-center justify-center py-2">
			{isFetchingNextPage && (
				<Spinner className="size-4 text-content-secondary" loading />
			)}
		</div>
	);
};
