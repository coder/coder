import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import { Link } from "react-router";
import type { GroupsByUserId } from "#/api/queries/groups";
import type * as TypesGen from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { PremiumBadge } from "#/components/Badges/Badges";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { LastSeen } from "#/components/LastSeen/LastSeen";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { AISeatCell } from "#/modules/users/AISeatCell";
import { UserGroupsCell } from "#/modules/users/UserGroupsCell";
import {
	AiAddonHelpPopover,
	GroupsHelpPopover,
	RolesHelpPopover,
} from "#/modules/users/UserHelpPopovers";
import { UserRoleCell } from "#/modules/users/UserRoleCell";
import { cn } from "#/utils/cn";

dayjs.extend(relativeTime);

export type UsersTableProps = {
	// State
	isLoading: boolean;
	users: readonly TypesGen.User[] | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	showAISeatColumn?: boolean;

	// Actions
	onEditUserRoles: (user: TypesGen.User) => void;
	isUpdatingUserRoles?: boolean;
	onResetUserPassword: (user: TypesGen.User) => void;
	onSuspendUser: (user: TypesGen.User) => void;
	onActivateUser: (user: TypesGen.User) => void;
	onDeleteUser: (user: TypesGen.User) => void;

	// Permissions
	/**
	 * Used to disable the UI of actions that users cannot perform on themselves,
	 * like delete.
	 */
	me: string;
	canEditUsers: boolean;
	canViewActivity?: boolean;
	/** User roles cannot be edited if OIDC Role Sync is enabled. */
	oidcRoleSyncEnabled?: boolean;
};

export const UsersTable: React.FC<UsersTableProps> = (props) => {
	const { showAISeatColumn } = props;

	return (
		<Table data-testid="users-table">
			<TableHeader>
				<TableRow>
					<TableHead className="w-max">User</TableHead>
					<TableHead className="w-1/6">
						<div className="flex flex-row gap-2 items-center">
							<span>Roles</span>
							<RolesHelpPopover />
						</div>
					</TableHead>
					<TableHead className="w-1/6">
						<div className="flex flex-row gap-2 items-center">
							<span>Groups</span>
							<GroupsHelpPopover />
						</div>
					</TableHead>
					{showAISeatColumn && (
						<TableHead className="w-1/6">
							<div className="flex flex-row gap-2 items-center">
								<span>AI add-on</span>
								<AiAddonHelpPopover />
							</div>
						</TableHead>
					)}
					<TableHead className="w-1/6">Status</TableHead>
				</TableRow>
			</TableHeader>

			<TableBody>
				<UsersTableBody {...props} />
			</TableBody>
		</Table>
	);
};

const UsersTableBody: React.FC<UsersTableProps> = ({
	isLoading,
	users,
	groupsByUserId,
	showAISeatColumn,

	onEditUserRoles,
	isUpdatingUserRoles,
	onResetUserPassword,
	onSuspendUser,
	onActivateUser,
	onDeleteUser,

	me,
	canEditUsers,
	canViewActivity,
	oidcRoleSyncEnabled,
}) => {
	if (isLoading) {
		return (
			<UsersTableSkeleton
				showAISeatColumn={showAISeatColumn}
				canEditUsers={canEditUsers}
			/>
		);
	}

	if (!users || users.length === 0) {
		return (
			<TableRow>
				<TableCell colSpan={999}>
					<div className="p-8">
						<EmptyState message="No users found" />
					</div>
				</TableCell>
			</TableRow>
		);
	}

	return (
		<>
			{users?.map((user) => (
				<TableRow key={user.id} data-testid={`user-${user.id}`}>
					<TableCell>
						<AvatarData
							title={user.username}
							subtitle={
								user.is_service_account ? "Service Account" : user.email
							}
							src={user.avatar_url}
						/>
					</TableCell>

					<UserRoleCell roles={user.roles} />

					<UserGroupsCell userGroups={groupsByUserId?.get(user.id)} />

					{showAISeatColumn && <AISeatCell hasAISeat={user.has_ai_seat} />}

					<TableCell
						className={cn(
							"capitalize",
							user.status === "suspended" && "text-content-secondary",
						)}
					>
						<div>{user.status}</div>
						{(user.status === "active" || user.status === "dormant") && (
							<LastSeen at={user.last_seen_at} className="text-xs" />
						)}
					</TableCell>

					{canEditUsers && (
						<TableCell>
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button
										size="icon-lg"
										variant="subtle"
										aria-label="Open menu"
									>
										<EllipsisVerticalIcon aria-hidden="true" />
										<span className="sr-only">Open menu</span>
									</Button>
								</DropdownMenuTrigger>

								<DropdownMenuContent align="end">
									<DropdownMenuItem asChild>
										<Link
											to={`/workspaces?filter=${encodeURIComponent(`owner:${user.username}`)}`}
										>
											View workspaces
										</Link>
									</DropdownMenuItem>

									{canViewActivity && (
										<DropdownMenuItem asChild disabled={!canViewActivity}>
											<Link
												to={`/audit?filter=${encodeURIComponent(`username:${user.username}`)}`}
											>
												View activity {!canViewActivity && <PremiumBadge />}
											</Link>
										</DropdownMenuItem>
									)}

									<DropdownMenuItem asChild>
										<Link to={user.username}>Edit</Link>
									</DropdownMenuItem>

									<DropdownMenuItem
										disabled={
											isUpdatingUserRoles ||
											(user.login_type === "oidc" && oidcRoleSyncEnabled)
										}
										onClick={() => onEditUserRoles(user)}
									>
										Edit roles
									</DropdownMenuItem>

									{user.status !== "suspended" && (
										<DropdownMenuItem
											disabled={user.login_type !== "password"}
											onClick={() => onResetUserPassword(user)}
										>
											Reset password&hellip;
										</DropdownMenuItem>
									)}

									{user.status === "active" || user.status === "dormant" ? (
										<DropdownMenuItem
											data-testid="suspend-button"
											onClick={() => onSuspendUser(user)}
										>
											Suspend&hellip;
										</DropdownMenuItem>
									) : (
										<DropdownMenuItem onClick={() => onActivateUser(user)}>
											Activate&hellip;
										</DropdownMenuItem>
									)}

									<DropdownMenuSeparator />

									<DropdownMenuItem
										className="text-content-destructive focus:text-content-destructive"
										onClick={() => onDeleteUser(user)}
										disabled={user.id === me}
									>
										<TrashIcon className="size-icon-xs" />
										Delete&hellip;
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						</TableCell>
					)}
				</TableRow>
			))}
		</>
	);
};

type UsersTableSkeletonProps = {
	showAISeatColumn?: boolean;
	canEditUsers: boolean;
};

const UsersTableSkeleton: React.FC<UsersTableSkeletonProps> = ({
	showAISeatColumn,
	canEditUsers,
}) => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<AvatarDataSkeleton />
				</TableCell>

				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>

				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>

				{showAISeatColumn && (
					<TableCell>
						<Skeleton variant="text" width="25%" />
					</TableCell>
				)}

				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>

				{canEditUsers && (
					<TableCell>
						<Skeleton variant="text" width="25%" />
					</TableCell>
				)}
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
