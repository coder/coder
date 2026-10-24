import { useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { groupsByUserId } from "#/api/queries/groups";
import { paginatedUsers } from "#/api/queries/users";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { useFilter, useFilterParamsKey } from "#/components/Filter/Filter";
import { parseFilterQuery } from "#/components/Filter/filterQuery";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import {
	LAST_SEEN_PRESET_PARAM,
	lastSeenUrlState,
	resolveLastSeen,
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

	const filterQuery = searchParams.get(useFilterParamsKey) ?? "";
	// Anchors preset ranges so the query key stays stable across renders.
	const [now, setNow] = useState(() => new Date());
	const lastSeen = resolveLastSeen(
		searchParams.get(LAST_SEEN_PRESET_PARAM),
		parseFilterQuery(filterQuery),
		now,
	);

	const usersQuery = usePaginatedQuery(
		paginatedUsers(searchParams, withLastSeen(filterQuery, lastSeen)),
	);
	const useFilterResult = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: usersQuery.goToFirstPage,
	});

	const onLastSeenChange = (value: DateTimeRangeValue) => {
		setNow(new Date());
		const { filter, preset } = lastSeenUrlState(useFilterResult.query, value);
		if (preset === undefined) {
			searchParams.delete(LAST_SEEN_PRESET_PARAM);
		} else {
			searchParams.set(LAST_SEEN_PRESET_PARAM, preset);
		}
		if (filter === useFilterResult.query) {
			usersQuery.goToFirstPage();
		} else {
			useFilterResult.update(filter);
		}
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
