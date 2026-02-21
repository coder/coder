import { paginatedInterceptions } from "api/queries/aiBridge";
import { useFilter } from "components/Filter/Filter";
import { useUserFilterMenu } from "components/Filter/UserFilter";
import { useAuthenticated } from "hooks";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "utils/page";
import { useClientFilterMenu } from "./RequestLogsFilter/ClientFilter";
import { useModelFilterMenu } from "./RequestLogsFilter/ModelFilter";
import { useProviderFilterMenu } from "./RequestLogsFilter/ProviderFilter";
import { RequestLogsPageView } from "./RequestLogsPageView";

const RequestLogsPage: FC = () => {
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	// Users are allowed to view their own request logs via the API,
	// but this page is only visible if the feature is enabled and the user
	// has the `viewAnyAIBridgeInterception` permission.
	// (as its defined in the Admin settings dropdown).
	const isEntitled = Boolean(feats.aibridge);
	const hasPermission = permissions.viewAnyAIBridgeInterception;
	const canViewRequestLogs = isEntitled && hasPermission;

	const [searchParams, setSearchParams] = useSearchParams();
	const interceptionsQuery = usePaginatedQuery({
		...paginatedInterceptions(searchParams),
		enabled: canViewRequestLogs,
	});
	const filter = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: interceptionsQuery.goToFirstPage,
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

	const modelMenu = useModelFilterMenu({
		value: filter.values.model,
		onChange: (option) =>
			filter.update({
				...filter.values,
				model: option?.value,
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

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Request Logs", "AI Bridge")}</title>

			<RequestLogsPageView
				isLoading={interceptionsQuery.isLoading}
				isRequestLogsVisible={isEntitled}
				interceptions={interceptionsQuery.data?.results}
				interceptionsQuery={interceptionsQuery}
				filterProps={{
					filter,
					error: interceptionsQuery.error,
					menus: {
						user: userMenu,
						provider: providerMenu,
						model: modelMenu,
						client: clientMenu,
					},
				}}
			/>
		</RequirePermission>
	);
};

export default RequestLogsPage;
