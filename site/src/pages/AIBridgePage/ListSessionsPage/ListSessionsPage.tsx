import type { FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { paginatedSessions } from "#/api/queries/aiBridge";
import { dataProtectionStatus } from "#/api/queries/deployment";
import { DataProtectionBanner } from "#/components/DataProtectionBanner";
import { useFilter } from "#/components/Filter/Filter";
import { useUserFilterMenu } from "#/components/Filter/UserFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { getAIBridgePermissions } from "../getAIBridgePermissions";
import { useClientFilterMenu } from "../RequestLogsPage/RequestLogsFilter/ClientFilter";
import { useModelFilterMenu } from "../RequestLogsPage/RequestLogsFilter/ModelFilter";
import { useProviderFilterMenu } from "../RequestLogsPage/RequestLogsFilter/ProviderFilter";
import { ListSessionsPageView } from "./ListSessionsPageView";

const AISessionListPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const navigate = useNavigate();

	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);

	const canViewSessions = isEntitled && hasPermission;

	const dpStatus = useQuery(dataProtectionStatus());
	const dataProtectionEnabled = dpStatus.data?.enabled;
	const isAuditor = dpStatus.data?.auditor;

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
		enabled: !dataProtectionEnabled,
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
			<title>{pageTitle("Sessions", "AI Bridge")}</title>

			<DataProtectionBanner
				dataProtectionEnabled={dataProtectionEnabled}
				isAuditor={isAuditor}
			/>

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
						user: dataProtectionEnabled ? undefined : userMenu,
						provider: providerMenu,
						client: clientMenu,
						model: modelMenu,
					},
				}}
			/>
		</RequirePermission>
	);
};

export default AISessionListPage;
