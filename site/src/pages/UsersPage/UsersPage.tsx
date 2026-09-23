import { useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { groupsByUserId } from "#/api/queries/groups";
import { paginatedUsers } from "#/api/queries/users";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { useFilter } from "#/components/Filter/Filter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import {
	ALL_TIME_PRESET_ID,
	parseLastSeenRange,
	withLastSeen,
} from "./filter/lastSeenRange";
import { UsersPageView } from "./UsersPageView";

const UsersPage: React.FC = () => {
	const [searchParams, setSearchParams] = useSearchParams();
	const { entitlements } = useDashboard();

	const groupsByUserIdQuery = useQuery(groupsByUserId());

	const { permissions, user: me } = useAuthenticated();
	const {
		createUser: canCreateUser,
		updateUsers: canEditUsers,
		viewDeploymentConfig,
	} = permissions;
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: viewDeploymentConfig,
	});

	const usersQuery = usePaginatedQuery(paginatedUsers(searchParams));
	const useFilterResult = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: usersQuery.goToFirstPage,
	});

	const lastSeenRange = parseLastSeenRange(useFilterResult.values);
	// The URL stores resolved timestamps, so the preset label only shows while
	// the URL range still matches the last picked preset.
	const [lastPicked, setLastPicked] = useState<DateTimeRangeValue>();
	const lastSeen: DateTimeRangeValue =
		lastSeenRange === undefined
			? { start: new Date(0), end: new Date(), preset: ALL_TIME_PRESET_ID }
			: {
					...lastSeenRange,
					preset:
						lastPicked?.start.getTime() === lastSeenRange.start.getTime() &&
						lastPicked.end.getTime() === lastSeenRange.end.getTime()
							? lastPicked.preset
							: undefined,
				};
	const onLastSeenChange = (value: DateTimeRangeValue) => {
		setLastPicked(value);
		useFilterResult.update(withLastSeen(useFilterResult.query, value));
	};

	// Indicates if oidc roles are synced from the oidc idp.
	// Assign 'false' if unknown.
	const oidcRoleSyncEnabled =
		viewDeploymentConfig &&
		deploymentValues?.config.oidc?.user_role_field !== "";

	const isLoading = usersQuery.isLoading || groupsByUserIdQuery.isLoading;

	return (
		<>
			<title>{pageTitle("Users")}</title>

			<UsersPageView
				isLoading={isLoading}
				filterProps={{
					filter: useFilterResult,
					error: usersQuery.error,
					lastSeen,
					onLastSeenChange,
				}}
				usersQuery={usersQuery}
				groupsByUserId={groupsByUserIdQuery.data}
				me={me.id}
				canCreateUser={canCreateUser}
				canEditUsers={canEditUsers}
				canViewActivity={entitlements.features.audit_log.enabled}
				oidcRoleSyncEnabled={oidcRoleSyncEnabled}
			/>
		</>
	);
};

export default UsersPage;
