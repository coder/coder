import type { FC } from "react";
import type { GroupsByUserId } from "#/api/queries/groups";
import type * as TypesGen from "#/api/typesGenerated";
import { Stack } from "#/components/Stack/Stack";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableColumnHelpPopover } from "../../OrganizationSettingsPage/UserTable/TableColumnHelpPopover";
import { UsersTableBody } from "./UsersTableBody";

interface UsersTableProps {
	users: readonly TypesGen.User[] | undefined;
	roles: TypesGen.AssignableRoles[] | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	isUpdatingUserRoles?: boolean;
	canEditUsers: boolean;
	canViewActivity?: boolean;
	showAISeatColumn?: boolean;
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
	showAISeatColumn,
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
					<TableHead className="w-2/6">User</TableHead>
					<TableHead className="w-2/6">
						<Stack direction="row" spacing={1} alignItems="center">
							<span>Roles</span>
							<TableColumnHelpPopover variant="roles" />
						</Stack>
					</TableHead>
					<TableHead className="w-1/6">
						<Stack direction="row" spacing={1} alignItems="center">
							<span>Groups</span>
							<TableColumnHelpPopover variant="groups" />
						</Stack>
					</TableHead>
					{showAISeatColumn && (
						<TableHead className="w-1/6">
							<Stack direction="row" spacing={1} alignItems="center">
								<span>AI add-on</span>
								<TableColumnHelpPopover variant="ai_addon" />
							</Stack>
						</TableHead>
					)}
					<TableHead className="w-1/6">Login Type</TableHead>
					<TableHead className="w-1/6">Status</TableHead>
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
					showAISeatColumn={showAISeatColumn}
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
