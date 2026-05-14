import {
	EllipsisVerticalIcon,
	TriangleAlertIcon,
	UserPlusIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { workspaceSharingSettings } from "#/api/queries/organizations";
import type {
	Group,
	TemplateACL,
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getGroupSubtitle } from "#/modules/groups";

/**
 * Checks whether a user or group has access to a template based on its ACL.
 * Returns true (assumes access) when template ACL data is unavailable.
 */
function hasTemplateAccess(
	entityId: string,
	entityType: "user" | "group",
	tplACL: TemplateACL | undefined,
	organizationId: string,
): boolean {
	// No template ACL data available (still loading, or the caller lacks
	// permission to read it). Assume access so we do not show false warnings.
	if (!tplACL) {
		return true;
	}

	// The "everyone" group has the same ID as the organization. When it
	// appears in the template's group ACL, every org member has access.
	const everyoneHasAccess = tplACL.group.some((g) => g.id === organizationId);
	if (everyoneHasAccess) {
		return true;
	}

	if (entityType === "group") {
		return tplACL.group.some((g) => g.id === entityId);
	}

	// For users, check direct user ACL. This does not cover indirect access
	// through group membership; the warning text accounts for that.
	return tplACL.users.some((u) => u.id === entityId);
}

const NoTemplateAccessIcon: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<TriangleAlertIcon
					className="size-4 text-content-warning shrink-0"
					aria-label="No template access"
				/>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">
				This member may not have access to this workspace's template. They will
				not be able to view or use this workspace until a template admin grants
				them access.
			</TooltipContent>
		</Tooltip>
	);
};

interface RoleSelectProps {
	value: WorkspaceRole;
	disabled?: boolean;
	onValueChange: (value: WorkspaceRole) => void;
}

const RoleSelect: FC<RoleSelectProps> = ({
	value,
	disabled,
	onValueChange,
}) => {
	const roleLabels: Record<WorkspaceRole, string> = {
		use: "Use",
		admin: "Admin",
		"": "",
	};

	return (
		<Select value={value} onValueChange={onValueChange} disabled={disabled}>
			<SelectTrigger className="w-40 h-auto">
				<SelectValue>
					<span className="bg-surface-secondary rounded-md px-3 py-0.5 inline-block">
						{roleLabels[value]}
					</span>
				</SelectValue>
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="use" className="flex-col items-start py-2 w-64">
					<div className="font-medium text-content-primary">Use</div>
					<div className="text-xs text-content-secondary leading-snug mt-0.5">
						Can read, access, start, and stop this workspace.
					</div>
				</SelectItem>
				<SelectItem value="admin" className="flex-col items-start py-2 w-64">
					<div className="font-medium text-content-primary">Admin</div>
					<div className="text-xs text-content-secondary leading-snug mt-0.5">
						Can manage workspace metadata, permissions, and settings.
					</div>
				</SelectItem>
			</SelectContent>
		</Select>
	);
};

type AddWorkspaceMemberFormProps = {
	isLoading: boolean;
	onSubmit: () => void;
	disabled: boolean;
	children: ReactNode;
};

export const AddWorkspaceMemberForm: FC<AddWorkspaceMemberFormProps> = ({
	isLoading,
	onSubmit,
	disabled,
	children,
}) => {
	return (
		<form action={onSubmit}>
			<div className="flex flex-row items-center gap-2">
				{children}
				<Button disabled={disabled || isLoading} type="submit">
					<Spinner loading={isLoading}>
						<UserPlusIcon className="size-icon-sm" />
					</Spinner>
					Add member
				</Button>
			</div>
		</form>
	);
};

type RoleSelectFieldProps = {
	value: WorkspaceRole;
	onChange: (value: WorkspaceRole) => void;
	disabled?: boolean;
};

export const RoleSelectField: FC<RoleSelectFieldProps> = ({
	value,
	onChange,
	disabled,
}) => {
	return (
		<Select
			value={value}
			onValueChange={(val: WorkspaceRole) => onChange(val)}
			disabled={disabled}
		>
			<SelectTrigger className="w-40">
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="use">Use</SelectItem>
				<SelectItem value="admin">Admin</SelectItem>
			</SelectContent>
		</Select>
	);
};

interface WorkspaceSharingFormProps {
	organizationId: string;
	workspaceACL: WorkspaceACL | undefined;
	templateACL?: TemplateACL;
	canUpdatePermissions: boolean;
	error: unknown;
	onUpdateUser: (user: WorkspaceUser, role: WorkspaceRole) => void;
	updatingUserId: WorkspaceUser["id"] | undefined;
	onRemoveUser: (user: WorkspaceUser) => void;
	onUpdateGroup: (group: WorkspaceGroup, role: WorkspaceRole) => void;
	updatingGroupId?: WorkspaceGroup["id"] | undefined;
	onRemoveGroup: (group: Group) => void;
	addMemberForm?: ReactNode;
	isCompact?: boolean;
	showRestartWarning?: boolean;
}

export const WorkspaceSharingForm: FC<WorkspaceSharingFormProps> = ({
	organizationId,
	workspaceACL,
	templateACL,
	canUpdatePermissions,
	error,
	updatingUserId,
	onUpdateUser,
	onRemoveUser,
	updatingGroupId,
	onUpdateGroup,
	onRemoveGroup,
	addMemberForm,
	isCompact,
	showRestartWarning,
}) => {
	const sharingSettingsQuery = useQuery(
		workspaceSharingSettings(organizationId),
	);

	if (sharingSettingsQuery.isLoading) {
		return (
			<TableBody>
				<TableLoader />
			</TableBody>
		);
	}

	if (!sharingSettingsQuery.data) {
		return (
			<TableBody>
				<TableRow>
					<TableCell colSpan={999}>
						<ErrorAlert error={sharingSettingsQuery.error} />
					</TableCell>
				</TableRow>
			</TableBody>
		);
	}

	if (sharingSettingsQuery.data.sharing_disabled) {
		return (
			<TableBody>
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="This workspace cannot be shared"
							description="Workspace sharing has been disabled for this organization."
							isCompact={isCompact}
						/>
					</TableCell>
				</TableRow>
			</TableBody>
		);
	}

	const isEmpty = Boolean(
		workspaceACL &&
			workspaceACL.users.length === 0 &&
			workspaceACL.group.length === 0,
	);

	// Determine which members lack template access so we can show warnings.
	const usersWithoutAccess = workspaceACL
		? workspaceACL.users.filter(
				(u) => !hasTemplateAccess(u.id, "user", templateACL, organizationId),
			)
		: [];
	const groupsWithoutAccess = workspaceACL
		? workspaceACL.group.filter(
				(g) => !hasTemplateAccess(g.id, "group", templateACL, organizationId),
			)
		: [];
	const hasAccessWarnings =
		usersWithoutAccess.length > 0 || groupsWithoutAccess.length > 0;

	const userLacksAccess = new Set(usersWithoutAccess.map((u) => u.id));
	const groupLacksAccess = new Set(groupsWithoutAccess.map((g) => g.id));

	const tableHeader = (
		<TableHeader>
			<TableRow>
				<TableHead className="w-[50%] py-2">Member</TableHead>
				<TableHead className="w-[40%] py-2">Role</TableHead>
				<TableHead className="w-[10%] py-2" />
			</TableRow>
		</TableHeader>
	);

	const tableBody = (
		<TableBody>
			{!workspaceACL ? (
				<TableLoader />
			) : isEmpty ? (
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="No shared members or groups yet"
							description="Add a member or group using the controls above."
							isCompact={isCompact}
						/>
					</TableCell>
				</TableRow>
			) : (
				<>
					{workspaceACL.group.map((group) => (
						<TableRow key={group.id}>
							<TableCell className="py-2 w-[50%]">
								<div className="flex items-center gap-2">
									<AvatarData
										avatar={
											<Avatar
												size="lg"
												fallback={group.display_name || group.name}
												src={group.avatar_url}
											/>
										}
										title={group.display_name || group.name}
										subtitle={getGroupSubtitle(group)}
									/>
									{groupLacksAccess.has(group.id) && <NoTemplateAccessIcon />}
								</div>
							</TableCell>
							<TableCell className="py-2 w-[40%]">
								{canUpdatePermissions ? (
									<RoleSelect
										value={group.role}
										disabled={updatingGroupId === group.id}
										onValueChange={(value) => onUpdateGroup(group, value)}
									/>
								) : (
									<div className="capitalize">{group.role}</div>
								)}
							</TableCell>

							<TableCell className="py-2 w-[10%]">
								{canUpdatePermissions && (
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
											<DropdownMenuItem
												className="text-content-destructive focus:text-content-destructive"
												onClick={() => onRemoveGroup(group)}
											>
												Remove
											</DropdownMenuItem>
										</DropdownMenuContent>
									</DropdownMenu>
								)}
							</TableCell>
						</TableRow>
					))}

					{workspaceACL.users.map((user) => (
						<TableRow key={user.id}>
							<TableCell className="py-2 w-[50%]">
								<div className="flex items-center gap-2">
									<AvatarData
										title={user.username}
										subtitle={user.name}
										src={user.avatar_url}
									/>
									{userLacksAccess.has(user.id) && <NoTemplateAccessIcon />}
								</div>
							</TableCell>
							<TableCell className="py-2 w-[40%]">
								{canUpdatePermissions ? (
									<RoleSelect
										value={user.role}
										disabled={updatingUserId === user.id}
										onValueChange={(value) => onUpdateUser(user, value)}
									/>
								) : (
									<div className="capitalize">{user.role}</div>
								)}
							</TableCell>

							<TableCell className="py-2 w-[10%]">
								{canUpdatePermissions && (
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
											<DropdownMenuItem
												className="text-content-destructive focus:text-content-destructive"
												onClick={() => onRemoveUser(user)}
											>
												Remove
											</DropdownMenuItem>
										</DropdownMenuContent>
									</DropdownMenu>
								)}
							</TableCell>
						</TableRow>
					))}
				</>
			)}
		</TableBody>
	);

	const warnings = (
		<>
			{hasAccessWarnings && (
				<Alert severity="warning">
					Some shared members may not have access to this workspace's template
					and will not be able to view or use this workspace until a template
					admin grants them access.
				</Alert>
			)}
		</>
	);

	if (isCompact) {
		return (
			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}
				{canUpdatePermissions && addMemberForm}
				{showRestartWarning && (
					<Alert severity="warning">
						Workspace restart required for the removal to take effect.
					</Alert>
				)}
				{warnings}
				<div>
					<Table>{tableHeader}</Table>
					<div className="max-h-60 overflow-y-auto">
						<Table>{tableBody}</Table>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			{Boolean(error) && <ErrorAlert error={error} />}
			{canUpdatePermissions && addMemberForm}
			{showRestartWarning && (
				<Alert severity="warning">
					Workspace restart required for the removal to take effect.
				</Alert>
			)}
			{warnings}
			<Table>
				{tableHeader}
				{tableBody}
			</Table>
		</div>
	);
};
