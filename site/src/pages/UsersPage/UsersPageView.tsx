import type { GroupsByUserId } from "api/queries/groups";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	PaginationContainer,
	type PaginationResult,
} from "components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { UserPlusIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { UsersFilter } from "./UsersFilter";
import { UsersTable } from "./UsersTable/UsersTable";

export interface UsersPageViewProps {
	users?: readonly TypesGen.User[];
	roles?: TypesGen.AssignableRoles[];
	isUpdatingUserRoles?: boolean;
	canEditUsers: boolean;
	oidcRoleSyncEnabled: boolean;
	canViewActivity?: boolean;
	isLoading: boolean;
	authMethods?: TypesGen.AuthMethods;
	onSuspendUser: (user: TypesGen.User) => void;
	onDeleteUser: (user: TypesGen.User) => void;
	onListWorkspaces: (user: TypesGen.User) => void;
	onViewActivity: (user: TypesGen.User) => void;
	onActivateUser: (user: TypesGen.User) => void;
	onResetUserPassword: (user: TypesGen.User) => void;
	onUpdateUserRoles: (
		userId: string,
		roles: TypesGen.SlimRole["name"][],
	) => void;
	filterProps: ComponentProps<typeof UsersFilter>;
	isNonInitialPage: boolean;
	actorID: string;
	groupsByUserId: GroupsByUserId | undefined;
	usersQuery: PaginationResult;

	// TODO: Refactor these out once we remove the multi-organization experiment.
	canViewOrganizations?: boolean;
	canCreateUser?: boolean;
}

export const UsersPageView: FC<UsersPageViewProps> = ({
	users,
	roles,
	onSuspendUser,
	onDeleteUser,
	onListWorkspaces,
	onViewActivity,
	onActivateUser,
	onResetUserPassword,
	onUpdateUserRoles,
	isUpdatingUserRoles,
	canEditUsers,
	oidcRoleSyncEnabled,
	canViewActivity,
	isLoading,
	filterProps,
	isNonInitialPage,
	actorID,
	authMethods,
	groupsByUserId,
	usersQuery,
	canCreateUser,
}) => {
	return (
		<>
			<SettingsHeader
				actions={
					canCreateUser && (
						<Button asChild>
							<RouterLink to="create">
								<UserPlusIcon />
								Create user
							</RouterLink>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>Users</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Manage user accounts and permissions.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<UsersFilter {...filterProps} />

			<PaginationContainer query={usersQuery} paginationUnitLabel="users">
				<UsersTable
					users={users}
					roles={roles}
					groupsByUserId={groupsByUserId}
					onSuspendUser={onSuspendUser}
					onDeleteUser={onDeleteUser}
					onListWorkspaces={onListWorkspaces}
					onViewActivity={onViewActivity}
					onActivateUser={onActivateUser}
					onResetUserPassword={onResetUserPassword}
					onUpdateUserRoles={onUpdateUserRoles}
					isUpdatingUserRoles={isUpdatingUserRoles}
					canEditUsers={canEditUsers}
					oidcRoleSyncEnabled={oidcRoleSyncEnabled}
					canViewActivity={canViewActivity}
					isLoading={isLoading}
					isNonInitialPage={isNonInitialPage}
					actorID={actorID}
					authMethods={authMethods}
				/>
			</PaginationContainer>
		</>
	);
};
