import type { FC } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { paginatedSessions } from "#/api/queries/aiBridge";
import { useFilter } from "#/components/Filter/Filter";
import { useUserFilterMenu } from "#/components/Filter/UserFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { useProviderFilterMenu } from "../RequestLogsPage/RequestLogsFilter/ProviderFilter";
import { ListSessionsPageView } from "./ListSessionsPageView";

const AISessionListPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const navigate = useNavigate();

	// Users are allowed to view their own request logs via the API,
	// but this page is only visible if the feature is enabled and the user
	// has the `viewAnyAIBridgeInterception` permission.
	// (as its defined in the Admin settings dropdown).
	const isEntitled =
		entitlements.features.aibridge.entitlement === "entitled" ||
		entitlements.features.aibridge.entitlement === "grace_period";
	const isEnabled = entitlements.features.aibridge.enabled;
	const hasPermission = permissions.viewAnyAIBridgeInterception;
	const canViewSessions = isEntitled && hasPermission;

	const [searchParams, setSearchParams] = useSearchParams();
	const sessionsQuery = usePaginatedQuery({
		...paginatedSessions(searchParams),
		enabled: canViewSessions,
	});
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: sessionsQuery.goToFirstPage,
	});

	const userMenu = useUserFilterMenu({
		value: filter.values.initiator,
		onChange: (option) =>
			filter.update({
				...filter.values,
				initiator: option?.value,
			}),
	});

	const providerMenu = useProviderFilterMenu({
		value: filter.values.provider,
		onChange: (option) =>
			filter.update({
				...filter.values,
				provider: option?.value,
			}),
	});

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Sessions", "AI Bridge")}</title>

			<ListSessionsPageView
				isLoading={sessionsQuery.isLoading}
				isFetching={sessionsQuery.isFetching}
				isAISessionsEntitled={isEntitled}
				isAISessionsEnabled={isEnabled}
				sessions={sessionsQuery.data?.sessions}
				sessionsQuery={sessionsQuery}
				onSessionRowClick={(sessionId) =>
					navigate(`/aibridge/sessions/${sessionId}`)
				}
				filterProps={{
					filter,
					error: sessionsQuery.error,
					menus: {
						user: userMenu,
						provider: providerMenu,
					},
				}}
			/>
		</RequirePermission>
	);
};

export default AISessionListPage;
