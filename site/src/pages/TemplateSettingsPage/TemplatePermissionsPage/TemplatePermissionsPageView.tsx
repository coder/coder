import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import type {
	Group,
	ReducedUser,
	TemplateACL,
	TemplateGroup,
	TemplateRole,
	TemplateUser,
} from "#/api/typesGenerated";
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
	PageHeader,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
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
import { getGroupSubtitle } from "#/modules/groups";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "./UserOrGroupAutocomplete";

type AddTemplateUserOrGroupProps = {
	templateID: string;
	isLoading: boolean;
	templateACL: TemplateACL | undefined;
	onSubmit: (
		userOrGroup:
			| TemplateUser
			| TemplateGroup
			// Reduce user is returned by the groups.
			| ({ role: TemplateRole } & ReducedUser),
		role: TemplateRole,
		reset: () => void,
	) => void;
};

const AddTemplateUserOrGroup: FC<AddTemplateUserOrGroupProps> = ({
	isLoading,
	templateID,
	templateACL,
	onSubmit,
}) => {
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);
	const [selectedRole, setSelectedRole] = useState<TemplateRole>("use");
	const excludeFromAutocomplete = templateACL
		? [...templateACL.group, ...templateACL.users]
		: [];

	const resetValues = () => {
		setSelectedOption(null);
		setSelectedRole("use");
	};

	return (
		<form
			onSubmit={(e) => {
				e.preventDefault();

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
			<div className="flex flex-row items-center gap-1">
				<UserOrGroupAutocomplete
					exclude={excludeFromAutocomplete}
					templateID={templateID}
					value={selectedOption}
					onChange={(newValue) => {
						setSelectedOption(newValue);
					}}
				/>

				<Select
					value={selectedRole}
					disabled={isLoading}
					onValueChange={(value) => {
						setSelectedRole(value as TemplateRole);
					}}
				>
					<SelectTrigger className="w-[100px]">
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
					Add
				</Button>
			</div>
		</form>
	);
};

interface RoleSelectProps {
	value: TemplateRole;
	disabled?: boolean;
	onValueChange: (value: TemplateRole) => void;
}

const RoleSelect: FC<RoleSelectProps> = ({
	value,
	disabled,
	onValueChange,
}) => {
	return (
		<Select
			value={value}
			disabled={disabled}
			onValueChange={(nextValue) => onValueChange(nextValue as TemplateRole)}
		>
			<SelectTrigger className="h-auto w-[200px]">
				<SelectValue>
					<span className="capitalize">{value}</span>
				</SelectValue>
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="use" className="w-[250px] flex-col items-start py-2">
					<div className="text-content-primary">Use</div>
					<div className="text-xs leading-[140%] text-content-secondary">
						Can read and use this template to create workspaces.
					</div>
				</SelectItem>
				<SelectItem
					value="admin"
					className="w-[250px] flex-col items-start py-2"
				>
					<div className="text-content-primary">Admin</div>
					<div className="text-xs leading-[140%] text-content-secondary">
						Can modify all aspects of this template including permissions,
						metadata, and template versions.
					</div>
				</SelectItem>
			</SelectContent>
		</Select>
	);
};

interface TemplatePermissionsPageViewProps {
	templateACL: TemplateACL | undefined;
	templateID: string;
	canUpdatePermissions: boolean;
	// User
	onAddUser: (
		user: TemplateUser | ({ role: TemplateRole } & ReducedUser),
		role: TemplateRole,
		reset: () => void,
	) => void;
	isAddingUser: boolean;
	onUpdateUser: (user: TemplateUser, role: TemplateRole) => void;
	updatingUserId: TemplateUser["id"] | undefined;
	onRemoveUser: (user: TemplateUser) => void;
	// Group
	onAddGroup: (
		group: TemplateGroup,
		role: TemplateRole,
		reset: () => void,
	) => void;
	isAddingGroup: boolean;
	onUpdateGroup: (group: TemplateGroup, role: TemplateRole) => void;
	updatingGroupId?: TemplateGroup["id"] | undefined;
	onRemoveGroup: (group: Group) => void;
}

export const TemplatePermissionsPageView: FC<
	TemplatePermissionsPageViewProps
> = ({
	templateACL,
	canUpdatePermissions,
	templateID,
	// User
	onAddUser,
	isAddingUser,
	updatingUserId,
	onUpdateUser,
	onRemoveUser,
	// Group
	onAddGroup,
	isAddingGroup,
	updatingGroupId,
	onUpdateGroup,
	onRemoveGroup,
}) => {
	return (
		<>
			<PageHeader className="pt-0">
				<PageHeaderTitle>Permissions</PageHeaderTitle>
			</PageHeader>

			<div className="flex flex-col gap-2.5">
				{canUpdatePermissions && (
					<AddTemplateUserOrGroup
						templateACL={templateACL}
						templateID={templateID}
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
							<TableHead className="w-[60%]">Member</TableHead>
							<TableHead className="w-[40%]">Role</TableHead>
							<TableHead className="w-[1%]" />
						</TableRow>
					</TableHeader>
					<TableBody>
						<MembersTableBody
							templateACL={templateACL}
							canUpdatePermissions={canUpdatePermissions}
							updatingUserId={updatingUserId}
							updatingGroupId={updatingGroupId}
							onUpdateUser={onUpdateUser}
							onRemoveUser={onRemoveUser}
							onUpdateGroup={onUpdateGroup}
							onRemoveGroup={onRemoveGroup}
						/>
					</TableBody>
				</Table>
			</div>
		</>
	);
};

interface MembersTableBodyProps {
	templateACL: TemplateACL | undefined;
	canUpdatePermissions: boolean;
	updatingUserId: TemplateUser["id"] | undefined;
	updatingGroupId: TemplateGroup["id"] | undefined;
	onUpdateUser: (user: TemplateUser, role: TemplateRole) => void;
	onRemoveUser: (user: TemplateUser) => void;
	onUpdateGroup: (group: TemplateGroup, role: TemplateRole) => void;
	onRemoveGroup: (group: Group) => void;
}

const MembersTableBody: FC<MembersTableBodyProps> = ({
	templateACL,
	canUpdatePermissions,
	updatingUserId,
	updatingGroupId,
	onUpdateUser,
	onRemoveUser,
	onUpdateGroup,
	onRemoveGroup,
}) => {
	if (!templateACL) {
		return <TableLoader />;
	}

	const isEmpty =
		templateACL.users.length === 0 && templateACL.group.length === 0;
	if (isEmpty) {
		return (
			<TableRow>
				<TableCell colSpan={999}>
					<EmptyState
						message="No members yet"
						description="Add a member using the controls above"
					/>
				</TableCell>
			</TableRow>
		);
	}

	return (
		<>
			{templateACL.group.map((group) => (
				<TableRow key={group.id}>
					<TableCell>
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
					<TableCell>
						{canUpdatePermissions ? (
							<RoleSelect
								value={group.role}
								disabled={updatingGroupId === group.id}
								onValueChange={(role) => {
									onUpdateGroup(group, role);
								}}
							/>
						) : (
							<div className="capitalize">{group.role}</div>
						)}
					</TableCell>

					<TableCell>
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

			{templateACL.users.map((user) => (
				<TableRow key={user.id}>
					<TableCell>
						<AvatarData
							title={user.username}
							subtitle={user.email}
							src={user.avatar_url}
						/>
					</TableCell>
					<TableCell>
						{canUpdatePermissions ? (
							<RoleSelect
								value={user.role}
								disabled={updatingUserId === user.id}
								onValueChange={(role) => {
									onUpdateUser(user, role);
								}}
							/>
						) : (
							<div className="capitalize">{user.role}</div>
						)}
					</TableCell>

					<TableCell>
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
	);
};
