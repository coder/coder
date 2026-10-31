import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { API } from "#/api/api";
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
	resolveLastSeen,
	resolveRelativeLastSeen,
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

	const usersQuery = usePaginatedQuery({
		...paginatedUsers(searchParams),
		// Relative last seen values stay in the query key, so it is stable across
		// renders, and each fetch resolves them against the time it runs.
		queryFn: ({ payload, signal }) =>
			API.getUsers(
				{ ...payload, q: resolveRelativeLastSeen(payload.q ?? "", new Date()) },
				signal,
			),
	});
	const useFilterResult = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
		onUpdate: usersQuery.goToFirstPage,
	});

	const lastSeen = resolveLastSeen(useFilterResult.values, new Date());
	const onLastSeenChange = (value: DateTimeRangeValue) => {
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
