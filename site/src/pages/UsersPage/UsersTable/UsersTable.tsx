import type { GroupsByUserId } from "api/queries/groups";
import type * as TypesGen from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { TableColumnHelpTooltip } from "../../OrganizationSettingsPage/UserTable/TableColumnHelpTooltip";
import { UsersTableBody } from "./UsersTableBody";

export const Language = {
	usernameLabel: "User",
	rolesLabel: "Roles",
	groupsLabel: "Groups",
	statusLabel: "Status",
	lastSeenLabel: "Last Seen",
	loginTypeLabel: "Login Type",
} as const;

export interface UsersTableProps {
	users: readonly TypesGen.User[] | undefined;
	roles: TypesGen.AssignableRoles[] | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	isUpdatingUserRoles?: boolean;
	canEditUsers: boolean;
	canViewActivity?: boolean;
	isLoading: boolean;
	onSuspendUser: (user: TypesGen.User) => void;
	onActivateUser: (user: TypesGen.User) => void;
	onDeleteUser: (user: TypesGen.User) => void;
	onListWorkspaces: (user: TypesGen.User) => void;
	onViewActivity: (user: TypesGen.User) => void;
	onResetUserPassword: (user: TypesGen.User) => void;
	onUpdateUserRoles: (
		userId: string,
		roles: TypesGen.SlimRole["name"][],
	) => void;
	isNonInitialPage: boolean;
	actorID: string;
	oidcRoleSyncEnabled: boolean;
	authMethods?: TypesGen.AuthMethods;
}

export const UsersTable: FC<UsersTableProps> = ({
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
	canViewActivity,
	isLoading,
	isNonInitialPage,
	actorID,
	oidcRoleSyncEnabled,
	authMethods,
	groupsByUserId,
}) => {
	return (
		<Table data-testid="users-table">
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/6">{Language.usernameLabel}</TableHead>
					<TableHead className="w-2/6">
						<Stack direction="row" spacing={1} alignItems="center">
							<span>{Language.rolesLabel}</span>
							<TableColumnHelpTooltip variant="roles" />
						</Stack>
					</TableHead>
					<TableHead className="w-1/6">
						<Stack direction="row" spacing={1} alignItems="center">
							<span>{Language.groupsLabel}</span>
							<TableColumnHelpTooltip variant="groups" />
						</Stack>
					</TableHead>
					<TableHead className="w-1/6">{Language.loginTypeLabel}</TableHead>
					<TableHead className="w-1/6">{Language.statusLabel}</TableHead>
					{canEditUsers && <TableHead className="w-auto" />}
				</TableRow>
			</TableHeader>

			<TableBody>
				<UsersTableBody
					users={users}
					roles={roles}
					groupsByUserId={groupsByUserId}
					isLoading={isLoading}
					canEditUsers={canEditUsers}
					canViewActivity={canViewActivity}
					isUpdatingUserRoles={isUpdatingUserRoles}
					onActivateUser={onActivateUser}
					onDeleteUser={onDeleteUser}
					onListWorkspaces={onListWorkspaces}
					onViewActivity={onViewActivity}
					onResetUserPassword={onResetUserPassword}
					onSuspendUser={onSuspendUser}
					onUpdateUserRoles={onUpdateUserRoles}
					isNonInitialPage={isNonInitialPage}
					actorID={actorID}
					oidcRoleSyncEnabled={oidcRoleSyncEnabled}
					authMethods={authMethods}
				/>
			</TableBody>
		</Table>
	);
};
