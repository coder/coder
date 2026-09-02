import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import { groupsByUserId } from "#/api/queries/groups";
import { paginatedUsers } from "#/api/queries/users";
import { useFilter } from "#/components/Filter/Filter";
import { useStatusFilterMenu } from "#/components/Filter/UsersFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
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

	const statusMenu = useStatusFilterMenu({
		value: useFilterResult.values.status,
		onChange: (option) =>
			useFilterResult.update({
				...useFilterResult.values,
				status: option?.value,
			}),
	});

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
					menus: { status: statusMenu },
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
