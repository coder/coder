import { EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import { Link } from "react-router";
import type { User } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import type { UserAdminAction } from "./UserActionDialogs";

type UserMoreActionsProps = {
	user: User;
	me: string;
	onAction: (action: UserAdminAction) => void;
	canViewActivity?: boolean;
	oidcRoleSyncEnabled?: boolean;
	showEdit?: boolean;
};

export const UserMoreActions: React.FC<UserMoreActionsProps> = ({
	user,
	me,
	onAction,
	canViewActivity,
	oidcRoleSyncEnabled,
	showEdit = true,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger
				render={
					<Button
						size="icon-lg"
						variant="subtle"
						type="button"
						aria-label="Open menu"
					/>
				}
			>
				<EllipsisVerticalIcon aria-hidden="true" />
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem
					render={
						<Link
							to={`/workspaces?filter=${encodeURIComponent(`owner:${user.username}`)}`}
						/>
					}
				>
					View workspaces
				</DropdownMenuItem>

				{canViewActivity && (
					<DropdownMenuItem
						render={
							<Link
								to={`/audit?filter=${encodeURIComponent(`username:${user.username}`)}`}
							/>
						}
					>
						View activity
					</DropdownMenuItem>
				)}

				{showEdit && (
					<DropdownMenuItem render={<Link to={user.username} />}>
						Edit
					</DropdownMenuItem>
				)}

				<DropdownMenuItem
					disabled={user.login_type === "oidc" && oidcRoleSyncEnabled}
					onClick={() => onAction({ type: "editRoles", user })}
				>
					Edit roles
				</DropdownMenuItem>

				{user.status !== "suspended" && (
					<DropdownMenuItem
						disabled={user.login_type !== "password"}
						onClick={() => onAction({ type: "resetPassword", user })}
					>
						Reset password&hellip;
					</DropdownMenuItem>
				)}

				{user.status === "active" || user.status === "dormant" ? (
					<DropdownMenuItem onClick={() => onAction({ type: "suspend", user })}>
						Suspend&hellip;
					</DropdownMenuItem>
				) : (
					<DropdownMenuItem
						onClick={() => onAction({ type: "activate", user })}
					>
						Activate&hellip;
					</DropdownMenuItem>
				)}

				<DropdownMenuSeparator />

				<DropdownMenuItem
					className="text-content-destructive focus:text-content-destructive"
					onClick={() => onAction({ type: "delete", user })}
					disabled={user.id === me}
				>
					<TrashIcon className="size-icon-xs" />
					Delete&hellip;
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
