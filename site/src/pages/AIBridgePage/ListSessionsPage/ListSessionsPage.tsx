import type { FC } from "react";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { paginatedSessions } from "#/api/queries/aiBridge";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { useFilter, useFilterParamsKey } from "#/components/Filter/Filter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { getAIBridgePermissions } from "../getAIBridgePermissions";
import { ListSessionsPageView } from "./ListSessionsPageView";
import {
	defaultTimeRange,
	parseTimeRange,
	queryWithTimeRange,
	withDefaultTimeRange,
} from "./timeRange";

const AISessionListPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const navigate = useNavigate();

	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);

	const canViewSessions = isEntitled && hasPermission;

	// The default time range lives in memory, not the URL, so a shared link
	// resolves relative to the viewer's current time. It is fixed per mount
	// so query cache keys stay stable.
	const [defaultRange] = useState(() => defaultTimeRange(new Date()));

	const [searchParams, setSearchParams] = useSearchParams();
	const sessionsQuery = usePaginatedQuery({
		...paginatedSessions(searchParams),
		// Merge the default range into every fetch (including prefetches) so
		// the unfiltered sessions query never scans the entire table.
		queryPayload: () =>
			withDefaultTimeRange(
				searchParams.get(useFilterParamsKey) ?? "",
				defaultRange,
			),
		enabled: canViewSessions,
	});

	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: sessionsQuery.goToFirstPage,
	});

	const explicitTimeRange = parseTimeRange(filter.values);
	const timeRange = explicitTimeRange ?? defaultRange;

	// The preset is display-only; the URL stores resolved timestamps. With no
	// range in the URL (e.g. after clearing the search) the default window
	// applies, which is the "Last 24 hours" preset. Otherwise show the last
	// picked preset only while the URL range still matches it.
	const [lastPicked, setLastPicked] = useState<DateTimeRangeValue | null>(null);
	// URL round-trips truncate to seconds, so compare at second precision.
	const sameSecond = (a: Date, b: Date) =>
		Math.floor(a.getTime() / 1000) === Math.floor(b.getTime() / 1000);
	const preset =
		explicitTimeRange === null
			? "last_24h"
			: lastPicked?.preset !== undefined &&
					sameSecond(lastPicked.start, timeRange.start) &&
					sameSecond(lastPicked.end, timeRange.end)
				? lastPicked.preset
				: undefined;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Sessions", "AI Gateway")}</title>

			<ListSessionsPageView
				isLoading={sessionsQuery.isLoading}
				isFetching={sessionsQuery.isFetching}
				isAISessionsEntitled={isEntitled}
				isAISessionsEnabled={isEnabled}
				sessions={sessionsQuery.data?.sessions}
				sessionsQuery={sessionsQuery}
				onSessionRowClick={(sessionId) =>
					navigate(`/ai-gateway/sessions/${sessionId}`)
				}
				filterProps={{
					filter,
					error: sessionsQuery.error,
					timeRange: {
						start: timeRange.start,
						end: timeRange.end,
						preset,
					},
					onTimeRangeChange: (value) => {
						setLastPicked(value);
						filter.update(queryWithTimeRange(filter.values, value));
					},
				}}
			/>
		</RequirePermission>
	);
};

export default AISessionListPage;
