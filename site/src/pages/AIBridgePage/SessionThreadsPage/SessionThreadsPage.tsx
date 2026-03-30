import type { FC } from "react";
import { useInfiniteQuery } from "react-query";
import { useParams } from "react-router";
import { infiniteSessionThreads } from "#/api/queries/aiBridge";
import { SessionThreadsPageView } from "./SessionThreadsPageView";

const SessionThreadsPage: FC = () => {
	const { sessionId } = useParams() as { sessionId: string };

	const sessionQuery = useInfiniteQuery({
		...infiniteSessionThreads(sessionId),
		enabled: !!sessionId,
	});

	const firstPage = sessionQuery.data?.pages[0];
	const allThreads =
		sessionQuery.data?.pages.flatMap((page) => page.threads) ?? [];

	return (
		<SessionThreadsPageView
			session={firstPage}
			threads={allThreads}
			loading={sessionQuery.isLoading}
			hasNextPage={sessionQuery.hasNextPage}
			isFetchingNextPage={sessionQuery.isFetchingNextPage}
			onFetchNextPage={sessionQuery.fetchNextPage}
		/>
	);
};

export default SessionThreadsPage;
