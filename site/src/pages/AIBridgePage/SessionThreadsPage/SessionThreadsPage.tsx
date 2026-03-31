import type { FC } from "react";
import { useInfiniteQuery } from "react-query";
import { useParams } from "react-router";
import { infiniteSessionThreads } from "#/api/queries/aiBridge";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { getAIBridgePermissions } from "../getAIBridgePermissions";
import { SessionThreadsPageView } from "./SessionThreadsPageView";

const SessionThreadsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();

	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);

	const canViewSessionThreads = isEntitled && hasPermission;

	const { sessionId } = useParams() as { sessionId: string };

	const sessionQuery = useInfiniteQuery({
		...infiniteSessionThreads(sessionId),
		enabled: canViewSessionThreads,
	});

	const firstPage = sessionQuery.data?.pages[0];
	const allThreads =
		sessionQuery.data?.pages.flatMap((page) => page.threads) ?? [];

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Session Threads", "AI Bridge")}</title>

			<SessionThreadsPageView
				session={firstPage}
				threads={allThreads}
				loading={sessionQuery.isLoading}
				hasNextPage={sessionQuery.hasNextPage}
				isFetchingNextPage={sessionQuery.isFetchingNextPage}
				onFetchNextPage={sessionQuery.fetchNextPage}
				isAISessionsEnabled={isEnabled}
				isAISessionsEntitled={isEntitled}
			/>
		</RequirePermission>
	);
};

export default SessionThreadsPage;
