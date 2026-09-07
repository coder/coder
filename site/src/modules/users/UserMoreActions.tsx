import { EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
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
};

export const UserMoreActions: FC<UserMoreActionsProps> = ({
	user,
	me,
	onAction,
	canViewActivity,
	oidcRoleSyncEnabled,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					size="icon-lg"
					variant="subtle"
					type="button"
					aria-label="Open menu"
				>
					<EllipsisVerticalIcon aria-hidden="true" />
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
					<DropdownMenuItem asChild>
						<Link
							to={`/audit?filter=${encodeURIComponent(`username:${user.username}`)}`}
						>
							View activity
						</Link>
					</DropdownMenuItem>
				)}

				<DropdownMenuItem asChild>
					<Link to={user.username}>Edit</Link>
				</DropdownMenuItem>

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
