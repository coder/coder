import type {
	Group,
	User,
	Workspace,
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
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
import { type FC, useState } from "react";
import { getGroupSubtitle } from "utils/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "./UserOrGroupAutocomplete";

type AddWorkspaceUserOrGroupProps = {
	organizationID: string;
	isLoading: boolean;
	workspaceACL: WorkspaceACL | undefined;
	onSubmit: (
		value: WorkspaceUser | Group | ({ role: WorkspaceRole } & User),
		role: WorkspaceRole,
		reset: () => void,
	) => void;
};

const AddWorkspaceUserOrGroup: FC<AddWorkspaceUserOrGroupProps> = ({
	organizationID,
	isLoading,
	workspaceACL,
	onSubmit,
}) => {
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);
	const [selectedRole, setSelectedRole] = useState<WorkspaceRole>("use");
	const excludeFromAutocomplete = workspaceACL
		? [...workspaceACL.group, ...workspaceACL.users]
		: [];

	const resetValues = () => {
		setSelectedOption(null);
		setSelectedRole("use");
	};

	return (
		<form
			onSubmit={(event) => {
				event.preventDefault();

				if (selectedOption && selectedRole) {
					onSubmit(
						{
							...selectedOption,
							role: selectedRole,
						},
						selectedRole,
						resetValues,
					);
				}
			}}
		>
			<div className="flex flex-row items-center gap-2">
				<UserOrGroupAutocomplete
					organizationId={organizationID}
					value={selectedOption}
					exclude={excludeFromAutocomplete}
					onChange={(newValue) => {
						setSelectedOption(newValue);
					}}
				/>

				<Select
					value={selectedRole}
					onValueChange={(value: WorkspaceRole) => setSelectedRole(value)}
					disabled={isLoading}
				>
					<SelectTrigger className="w-40">
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="use">Use</SelectItem>
						<SelectItem value="admin">Admin</SelectItem>
					</SelectContent>
				</Select>

				<Button
					disabled={!selectedRole || !selectedOption || isLoading}
					type="submit"
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
						Can read and access this workspace.
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

interface WorkspaceSharingPageViewProps {
	workspace: Workspace;
	workspaceACL: WorkspaceACL | undefined;
	canUpdatePermissions: boolean;
	onAddUser: (
		user: WorkspaceUser | ({ role: WorkspaceRole } & User),
		role: WorkspaceRole,
		reset: () => void,
	) => void;
	isAddingUser: boolean;
	onUpdateUser: (user: WorkspaceUser, role: WorkspaceRole) => void;
	updatingUserId: WorkspaceUser["id"] | undefined;
	onRemoveUser: (user: WorkspaceUser) => void;
	onAddGroup: (group: Group, role: WorkspaceRole, reset: () => void) => void;
	isAddingGroup: boolean;
	onUpdateGroup: (group: WorkspaceGroup, role: WorkspaceRole) => void;
	updatingGroupId?: WorkspaceGroup["id"] | undefined;
	onRemoveGroup: (group: Group) => void;
}

export const WorkspaceSharingPageView: FC<WorkspaceSharingPageViewProps> = ({
	workspace,
	workspaceACL,
	canUpdatePermissions,
	onAddUser,
	isAddingUser,
	updatingUserId,
	onUpdateUser,
	onRemoveUser,
	onAddGroup,
	isAddingGroup,
	updatingGroupId,
	onUpdateGroup,
	onRemoveGroup,
}) => {
	const isEmpty = Boolean(
		workspaceACL &&
			workspaceACL.users.length === 0 &&
			workspaceACL.group.length === 0,
	);

	return (
		<div className="flex flex-col gap-4">
			{canUpdatePermissions && (
				<AddWorkspaceUserOrGroup
					organizationID={workspace.organization_id}
					workspaceACL={workspaceACL}
					isLoading={isAddingUser || isAddingGroup}
					onSubmit={(value, role, resetAutocomplete) =>
						"members" in value
							? onAddGroup(value, role, resetAutocomplete)
							: onAddUser(value, role, resetAutocomplete)
					}
				/>
			)}
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[60%] py-2">Member</TableHead>
						<TableHead className="w-[40%] py-2">Role</TableHead>
						<TableHead className="w-[1%] py-2" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{!workspaceACL ? (
						<TableLoader />
					) : isEmpty ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState
									message="No shared members or groups yet"
									description="Add a member or group using the controls above"
								/>
							</TableCell>
						</TableRow>
					) : (
						<>
							{workspaceACL.group.map((group) => (
								<TableRow key={group.id}>
									<TableCell className="py-2">
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
									<TableCell className="py-2">
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

									<TableCell className="py-2">
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
									<TableCell className="py-2">
										<AvatarData
											title={user.username}
											subtitle={user.name}
											src={user.avatar_url}
										/>
									</TableCell>
									<TableCell className="py-2">
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

									<TableCell className="py-2">
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
			</Table>
		</div>
	);
};
