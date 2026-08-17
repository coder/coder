import type { FC } from "react";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { paginatedSessions } from "#/api/queries/aiBridge";
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
	setTimeRangeInQuery,
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
					timeRange,
					isDefaultTimeRange: explicitTimeRange === null,
					onTimeRangeChange: (range) =>
						filter.update(setTimeRangeInQuery(filter.values, range)),
				}}
			/>
		</RequirePermission>
	);
};

export default AISessionListPage;
