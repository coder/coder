import { cn } from "cn";
import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { workspaceSharingSettings } from "#/api/queries/organizations";
import type {
	Group,
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
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { getGroupSubtitle } from "#/modules/groups";

const roleLabels: Record<WorkspaceRole, string> = {
	use: "Use",
	admin: "Admin",
	"": "",
};

// Speech input users activate a control by its visible text, so the
// accessible name has to include the selected role.
const getRoleSelectLabel = (label: string, value: WorkspaceRole) =>
	roleLabels[value] ? `${label}: ${roleLabels[value]}` : label;

type RoleSelectProps = {
	value: WorkspaceRole;
	memberOrGroupName: string;
	isUpdating: boolean;
	onValueChange: (value: WorkspaceRole) => void;
};

const RoleSelect: React.FC<RoleSelectProps> = ({
	value,
	memberOrGroupName,
	isUpdating,
	onValueChange,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<Select
			value={value}
			onValueChange={(newValue: WorkspaceRole) => {
				if (!isUpdating) {
					onValueChange(newValue);
				}
			}}
			open={open}
			onOpenChange={(nextOpen) => setOpen(nextOpen && !isUpdating)}
		>
			<SelectTrigger
				className="w-40 h-auto aria-disabled:opacity-50"
				// Unlike disabled, aria-disabled keeps focus on the trigger while pending.
				aria-disabled={isUpdating || undefined}
				aria-label={getRoleSelectLabel(`Role for ${memberOrGroupName}`, value)}
			>
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
	children: React.ReactNode;
};

export const AddWorkspaceMemberForm: React.FC<AddWorkspaceMemberFormProps> = ({
	isLoading,
	onSubmit,
	disabled,
	children,
}) => {
	return (
		<form
			action={() => {
				if (!isLoading) {
					onSubmit();
				}
			}}
		>
			<div className="flex flex-col gap-2 sm:flex-row sm:items-center">
				{children}
				{/* aria-disabled, unlike disabled, keeps focus here while pending. */}
				<Button
					disabled={disabled}
					aria-disabled={isLoading || undefined}
					type="submit"
					variant="outline"
					className="w-full sm:w-auto"
				>
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

export const RoleSelectField: React.FC<RoleSelectFieldProps> = ({
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
			<SelectTrigger
				className="w-full sm:w-40"
				aria-label={getRoleSelectLabel("Role", value)}
			>
				<SelectValue />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="use">Use</SelectItem>
				<SelectItem value="admin">Admin</SelectItem>
			</SelectContent>
		</Select>
	);
};

const tableLabel = "Shared workspace members and groups";

type WorkspaceSharingFormProps = {
	organizationId: string;
	workspaceACL: WorkspaceACL | undefined;
	canShareWorkspace: boolean;
	error: unknown;
	onUpdateUser: (user: WorkspaceUser, role: WorkspaceRole) => void;
	updatingUserId: WorkspaceUser["id"] | undefined;
	onRemoveUser: (user: WorkspaceUser) => void;
	onUpdateGroup: (group: WorkspaceGroup, role: WorkspaceRole) => void;
	updatingGroupId?: WorkspaceGroup["id"] | undefined;
	onRemoveGroup: (group: Group) => void;
	addMemberForm?: React.ReactNode;
	isCompact?: boolean;
	showRestartWarning?: boolean;
};

export const WorkspaceSharingForm: React.FC<WorkspaceSharingFormProps> = ({
	organizationId,
	workspaceACL,
	canShareWorkspace,
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
			<Table aria-label={tableLabel}>
				<TableBody>
					<TableLoader />
				</TableBody>
			</Table>
		);
	}

	if (!sharingSettingsQuery.data) {
		return (
			<Table aria-label={tableLabel}>
				<TableBody>
					<TableRow>
						<TableCell colSpan={999}>
							<ErrorAlert error={sharingSettingsQuery.error} />
						</TableCell>
					</TableRow>
				</TableBody>
			</Table>
		);
	}

	if (sharingSettingsQuery.data.sharing_disabled) {
		return (
			<Table aria-label={tableLabel}>
				<TableBody>
					<TableEmpty
						message="This workspace cannot be shared"
						description="Workspace sharing has been disabled for this organization."
						isCompact={isCompact}
					/>
				</TableBody>
			</Table>
		);
	}

	const isEmpty = Boolean(
		workspaceACL &&
			workspaceACL.users.length === 0 &&
			workspaceACL.group.length === 0,
	);

	const tableHeader = (
		<TableHeader
			className={cn(isCompact && "sticky top-0 z-10 bg-surface-primary")}
		>
			<TableRow>
				<TableHead className="w-[50%] py-2">Member</TableHead>
				<TableHead className="w-[40%] py-2 max-sm:px-1">Role</TableHead>
				<TableHead className="w-[10%] py-2 max-sm:px-1">
					<span className="sr-only">Actions</span>
				</TableHead>
			</TableRow>
		</TableHeader>
	);

	const tableBody = (
		<TableBody>
			{!workspaceACL ? (
				<TableLoader />
			) : isEmpty ? (
				<TableEmpty
					message="No shared members or groups yet"
					description="Add a member or group using the controls above."
					isCompact={isCompact}
				/>
			) : (
				<>
					{workspaceACL.group.map((group) => (
						<TableRow key={group.id}>
							<TableCell className="py-2 w-[50%] max-sm:whitespace-nowrap">
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
							<TableCell className="py-2 w-[40%] max-sm:px-1">
								{canShareWorkspace ? (
									<RoleSelect
										value={group.role}
										memberOrGroupName={group.display_name || group.name}
										isUpdating={updatingGroupId === group.id}
										onValueChange={(value) => onUpdateGroup(group, value)}
									/>
								) : (
									<div className="capitalize">{group.role}</div>
								)}
							</TableCell>

							<TableCell className="py-2 w-[10%] max-sm:px-1">
								{canShareWorkspace && (
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button
												size="icon-lg"
												variant="subtle"
												aria-label={`Open menu for ${group.display_name || group.name}`}
											>
												<EllipsisVerticalIcon aria-hidden="true" />
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
							<TableCell className="py-2 w-[50%] max-sm:whitespace-nowrap">
								<AvatarData
									title={user.username}
									subtitle={user.name}
									src={user.avatar_url}
								/>
							</TableCell>
							<TableCell className="py-2 w-[40%] max-sm:px-1">
								{canShareWorkspace ? (
									<RoleSelect
										value={user.role}
										memberOrGroupName={user.username}
										isUpdating={updatingUserId === user.id}
										onValueChange={(value) => onUpdateUser(user, value)}
									/>
								) : (
									<div className="capitalize">{user.role}</div>
								)}
							</TableCell>

							<TableCell className="py-2 w-[10%] max-sm:px-1">
								{canShareWorkspace && (
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button
												size="icon-lg"
												variant="subtle"
												aria-label={`Open menu for ${user.username}`}
											>
												<EllipsisVerticalIcon aria-hidden="true" />
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

	return (
		<div className="flex flex-col gap-4">
			{Boolean(error) && <ErrorAlert error={error} />}
			{canShareWorkspace && addMemberForm}
			{showRestartWarning && (
				<Alert severity="warning">
					Workspace restart required for the removal to take effect.
				</Alert>
			)}
			{/* Screen readers associate headers with cells only within one table. */}
			<Table
				aria-label={tableLabel}
				wrapperClassName={cn(isCompact && "max-h-60")}
				// Without row controls, keyboard users need the scroll area itself to be focusable.
				wrapperProps={
					!canShareWorkspace
						? {
								tabIndex: 0,
								role: "region",
								"aria-label": tableLabel,
							}
						: undefined
				}
			>
				{tableHeader}
				{tableBody}
			</Table>
		</div>
	);
};
