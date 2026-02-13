import type {
	Group,
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableLoader } from "components/TableLoader/TableLoader";
import { EllipsisVertical, UserPlusIcon } from "lucide-react";
import { getGroupSubtitle } from "modules/groups";
import type { FC, ReactNode } from "react";

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
	workspaceACL: WorkspaceACL | undefined;
	canUpdatePermissions: boolean;
	isTaskWorkspace: boolean;
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
	workspaceACL,
	canUpdatePermissions,
	isTaskWorkspace,
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
	const isEmpty = Boolean(
		workspaceACL &&
			workspaceACL.users.length === 0 &&
			workspaceACL.group.length === 0,
	);

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
			{isTaskWorkspace ? (
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="Task workspaces cannot be shared"
							description="This workspace is managed by a task. Task sharing has not yet been implemented."
							isCompact={isCompact}
						/>
					</TableCell>
				</TableRow>
			) : !workspaceACL ? (
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
												<EllipsisVertical aria-hidden="true" />
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
								<AvatarData
									title={user.username}
									subtitle={user.name}
									src={user.avatar_url}
								/>
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
												<EllipsisVertical aria-hidden="true" />
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
			<Table>
				{tableHeader}
				{tableBody}
			</Table>
		</div>
	);
};
