import type { FC } from "react";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { paginatedSessions } from "#/api/queries/aiBridge";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { useFilter, useFilterParamsKey } from "#/components/Filter/Filter";
import { useUserFilterMenu } from "#/components/Filter/UserFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { useClientFilterMenu } from "../filters/ClientFilter";
import { useModelFilterMenu } from "../filters/ModelFilter";
import { useProviderFilterMenu } from "../filters/ProviderFilter";
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

	// The URL stores only resolved timestamps; a preset is display metadata
	// for the picker trigger. Remember the last committed picker value so the
	// preset label survives re-renders, but show it only while the URL range
	// still matches what the preset resolved to. The default window is the 24
	// hours ending at mount, which is exactly the "Last 24 hours" preset.
	const [lastPicked, setLastPicked] = useState<DateTimeRangeValue>(() => ({
		start: defaultRange.startedAfter,
		end: defaultRange.startedBefore,
		preset: "last_24h",
	}));
	// RFC 3339 serialization truncates to seconds, so ranges that round-trip
	// through the URL lose sub-second precision. Compare at second precision.
	const sameSecond = (a: Date, b: Date) =>
		Math.floor(a.getTime() / 1000) === Math.floor(b.getTime() / 1000);
	const preset =
		lastPicked.preset !== undefined &&
		sameSecond(lastPicked.start, timeRange.startedAfter) &&
		sameSecond(lastPicked.end, timeRange.startedBefore)
			? lastPicked.preset
			: undefined;

	const userMenu = useUserFilterMenu({
		value: filter.values.initiator,
		onChange: (option) =>
			filter.update({
				...filter.values,
				initiator: option?.value,
			}),
	});

	const providerMenu = useProviderFilterMenu({
		value: filter.values.provider_name,
		onChange: (option) =>
			filter.update({
				...filter.values,
				provider_name: option?.value,
			}),
	});

	const clientMenu = useClientFilterMenu({
		value: filter.values.client,
		onChange: (option) =>
			filter.update({
				...filter.values,
				client: option?.value,
			}),
	});

	const modelMenu = useModelFilterMenu({
		value: filter.values.model,
		onChange: (option) =>
			filter.update({
				...filter.values,
				model: option?.value,
			}),
	});

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
					menus: {
						user: userMenu,
						provider: providerMenu,
						client: clientMenu,
						model: modelMenu,
					},
					timeRange: {
						start: timeRange.startedAfter,
						end: timeRange.startedBefore,
						preset,
					},
					onTimeRangeChange: (value) => {
						setLastPicked(value);
						filter.update(
							queryWithTimeRange(filter.values, {
								startedAfter: value.start,
								startedBefore: value.end,
							}),
						);
					},
				}}
			/>
		</RequirePermission>
	);
};

export default AISessionListPage;
