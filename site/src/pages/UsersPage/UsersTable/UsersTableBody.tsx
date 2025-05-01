import type { Interpolation, Theme } from "@emotion/react";
import DeleteIcon from "@mui/icons-material/Delete";
import GitHub from "@mui/icons-material/GitHub";
import HideSourceOutlined from "@mui/icons-material/HideSourceOutlined";
import KeyOutlined from "@mui/icons-material/KeyOutlined";
import PasswordOutlined from "@mui/icons-material/PasswordOutlined";
import ShieldOutlined from "@mui/icons-material/ShieldOutlined";
import Skeleton from "@mui/material/Skeleton";
import type { GroupsByUserId } from "api/queries/groups";
import type * as TypesGen from "api/typesGenerated";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { PremiumBadge } from "components/Badges/Badges";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { LastSeen } from "components/LastSeen/LastSeen";
import { TableCell, TableRow } from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { EllipsisVertical } from "lucide-react";
import type { FC } from "react";
import { UserRoleCell } from "../../OrganizationSettingsPage/UserTable/UserRoleCell";
import { UserGroupsCell } from "./UserGroupsCell";

dayjs.extend(relativeTime);

interface UsersTableBodyProps {
	users: readonly TypesGen.User[] | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	authMethods?: TypesGen.AuthMethods;
	roles?: TypesGen.AssignableRoles[];
	isUpdatingUserRoles?: boolean;
	canEditUsers: boolean;
	isLoading: boolean;
	canViewActivity?: boolean;
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
	isNonInitialPage: boolean;
	actorID: string;
	// oidcRoleSyncEnabled should be set to false if unknown.
	// This is used to determine if the oidc roles are synced from the oidc idp and
	// editing via the UI should be disabled.
	oidcRoleSyncEnabled: boolean;
}

export const UsersTableBody: FC<UsersTableBodyProps> = ({
	users,
	authMethods,
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
	groupsByUserId,
}) => {
	return (
		<ChooseOne>
			<Cond condition={Boolean(isLoading)}>
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

						<TableCell>
							<Skeleton variant="text" width="25%" />
						</TableCell>

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
			</Cond>

			<Cond condition={!users || users.length === 0}>
				<ChooseOne>
					<Cond condition={isNonInitialPage}>
						<TableRow>
							<TableCell colSpan={999}>
								<div css={{ padding: 32 }}>
									<EmptyState message="No users found on this page" />
								</div>
							</TableCell>
						</TableRow>
					</Cond>

					<Cond>
						<TableRow>
							<TableCell colSpan={999}>
								<div css={{ padding: 32 }}>
									<EmptyState message="No users found" />
								</div>
							</TableCell>
						</TableRow>
					</Cond>
				</ChooseOne>
			</Cond>

			<Cond>
				{users?.map((user) => (
					<TableRow key={user.id} data-testid={`user-${user.id}`}>
						<TableCell>
							<AvatarData
								title={user.username}
								subtitle={user.email}
								src={user.avatar_url}
							/>
						</TableCell>

						<UserRoleCell
							canEditUsers={canEditUsers}
							allAvailableRoles={roles}
							userLoginType={user.login_type}
							roles={user.roles}
							oidcRoleSyncEnabled={oidcRoleSyncEnabled}
							isLoading={Boolean(isUpdatingUserRoles)}
							onEditRoles={(roles) => onUpdateUserRoles(user.id, roles)}
						/>

						<UserGroupsCell userGroups={groupsByUserId?.get(user.id)} />

						<TableCell>
							<LoginType authMethods={authMethods!} value={user.login_type} />
						</TableCell>

						<TableCell
							css={[
								styles.status,
								user.status === "suspended" && styles.suspended,
							]}
						>
							<div>{user.status}</div>
							{(user.status === "active" || user.status === "dormant") && (
								<LastSeen at={user.last_seen_at} css={{ fontSize: 12 }} />
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
											<EllipsisVertical aria-hidden="true" />
											<span className="sr-only">Open menu</span>
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
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

										<DropdownMenuItem onClick={() => onListWorkspaces(user)}>
											View workspaces
										</DropdownMenuItem>

										{canViewActivity && (
											<DropdownMenuItem
												onClick={() => onViewActivity(user)}
												disabled={!canViewActivity}
											>
												View activity {!canViewActivity && <PremiumBadge />}
											</DropdownMenuItem>
										)}

										{user.login_type === "password" && (
											<DropdownMenuItem
												onClick={() => onResetUserPassword(user)}
												disabled={user.login_type !== "password"}
											>
												Reset password&hellip;
											</DropdownMenuItem>
										)}

										<DropdownMenuSeparator />

										<DropdownMenuItem
											className="text-content-destructive focus:text-content-destructive"
											onClick={() => onDeleteUser(user)}
											disabled={user.id === actorID}
										>
											<DeleteIcon />
											Delete&hellip;
										</DropdownMenuItem>
									</DropdownMenuContent>
								</DropdownMenu>
							</TableCell>
						)}
					</TableRow>
				))}
			</Cond>
		</ChooseOne>
	);
};

interface LoginTypeProps {
	authMethods: TypesGen.AuthMethods;
	value: TypesGen.LoginType;
}

const LoginType: FC<LoginTypeProps> = ({ authMethods, value }) => {
	let displayName: string = value;
	let icon = <></>;

	if (value === "password") {
		displayName = "Password";
		icon = <PasswordOutlined css={styles.icon} />;
	} else if (value === "none") {
		displayName = "None";
		icon = <HideSourceOutlined css={styles.icon} />;
	} else if (value === "github") {
		displayName = "GitHub";
		icon = <GitHub css={styles.icon} />;
	} else if (value === "token") {
		displayName = "Token";
		icon = <KeyOutlined css={styles.icon} />;
	} else if (value === "oidc") {
		displayName =
			authMethods.oidc.signInText === "" ? "OIDC" : authMethods.oidc.signInText;
		icon =
			authMethods.oidc.iconUrl === "" ? (
				<ShieldOutlined css={styles.icon} />
			) : (
				<img
					alt="Open ID Connect icon"
					src={authMethods.oidc.iconUrl}
					css={styles.icon}
				/>
			);
	}

	return (
		<div css={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14 }}>
			{icon}
			{displayName}
		</div>
	);
};

const styles = {
	icon: {
		width: 14,
		height: 14,
	},

	status: {
		textTransform: "capitalize",
	},

	suspended: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
