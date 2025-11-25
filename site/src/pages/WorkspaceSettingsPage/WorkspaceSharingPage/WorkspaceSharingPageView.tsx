import type { Interpolation, Theme } from "@emotion/react";
import MenuItem from "@mui/material/MenuItem";
import Select, { type SelectProps } from "@mui/material/Select";
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
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
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
			<Stack direction="row" alignItems="center" spacing={1}>
				<UserOrGroupAutocomplete
					organizationId={organizationID}
					value={selectedOption}
					exclude={excludeFromAutocomplete}
					onChange={(newValue) => {
						setSelectedOption(newValue);
					}}
				/>

				<Select
					defaultValue="use"
					size="small"
					css={styles.select}
					disabled={isLoading}
					onChange={(event) => {
						setSelectedRole(event.target.value as WorkspaceRole);
					}}
				>
					<MenuItem key="use" value="use">
						Use
					</MenuItem>
					<MenuItem key="admin" value="admin">
						Admin
					</MenuItem>
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
			</Stack>
		</form>
	);
};

const RoleSelect: FC<SelectProps> = (props) => {
	return (
		<Select
			renderValue={(value) => <div css={styles.role}>{`${value}`}</div>}
			css={styles.updateSelect}
			{...props}
		>
			<MenuItem key="use" value="use" css={styles.menuItem}>
				<div>
					<div>Use</div>
					<div css={styles.menuItemSecondary}>
						Can read and access this workspace.
					</div>
				</div>
			</MenuItem>
			<MenuItem key="admin" value="admin" css={styles.menuItem}>
				<div>
					<div>Admin</div>
					<div css={styles.menuItemSecondary}>
						Can manage workspace metadata, permissions, and settings.
					</div>
				</div>
			</MenuItem>
		</Select>
	);
};

interface WorkspaceSharingPageViewProps {
	workspace: Workspace;
	workspaceACL: WorkspaceACL | undefined;
	canUpdatePermissions: boolean;
	// User
	onAddUser: (
		user: WorkspaceUser | ({ role: WorkspaceRole } & User),
		role: WorkspaceRole,
		reset: () => void,
	) => void;
	isAddingUser: boolean;
	onUpdateUser: (user: WorkspaceUser, role: WorkspaceRole) => void;
	updatingUserId: WorkspaceUser["id"] | undefined;
	onRemoveUser: (user: WorkspaceUser) => void;
	// Group
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
	const isEmpty = Boolean(
		workspaceACL &&
			workspaceACL.users.length === 0 &&
			workspaceACL.group.length === 0,
	);

	return (
		<>
			<PageHeader className="pt-0">
				<PageHeaderTitle>Sharing</PageHeaderTitle>
			</PageHeader>

			<Stack spacing={2.5}>
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
							<TableHead className="w-[60%]">Member</TableHead>
							<TableHead className="w-[40%]">Role</TableHead>
							<TableHead className="w-[1%]" />
						</TableRow>
					</TableHeader>
					<TableBody>
						<ChooseOne>
							<Cond condition={!workspaceACL}>
								<TableLoader />
							</Cond>
							<Cond condition={isEmpty}>
								<TableRow>
									<TableCell colSpan={999}>
										<EmptyState
											message="No shared members or groups yet"
											description="Add a member or group using the controls above"
										/>
									</TableCell>
								</TableRow>
							</Cond>
							<Cond>
								{workspaceACL?.group.map((group) => (
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
											<ChooseOne>
												<Cond condition={canUpdatePermissions}>
													<RoleSelect
														value={group.role}
														disabled={updatingGroupId === group.id}
														onChange={(event) => {
															onUpdateGroup(
																group,
																event.target.value as WorkspaceRole,
															);
														}}
													/>
												</Cond>
												<Cond>
													<div css={styles.role}>{group.role}</div>
												</Cond>
											</ChooseOne>
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

								{workspaceACL?.users.map((user) => (
									<TableRow key={user.id}>
										<TableCell>
											<AvatarData
												title={user.username}
												subtitle={user.name}
												src={user.avatar_url}
											/>
										</TableCell>
										<TableCell>
											<ChooseOne>
												<Cond condition={canUpdatePermissions}>
													<RoleSelect
														value={user.role}
														disabled={updatingUserId === user.id}
														onChange={(event) => {
															onUpdateUser(
																user,
																event.target.value as WorkspaceRole,
															);
														}}
													/>
												</Cond>
												<Cond>
													<div css={styles.role}>{user.role}</div>
												</Cond>
											</ChooseOne>
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
							</Cond>
						</ChooseOne>
					</TableBody>
				</Table>
			</Stack>
		</>
	);
};

const styles = {
	select: {
		fontSize: 14,
		width: 100,
	},
	updateSelect: {
		margin: 0,
		width: 200,
		"& .MuiSelect-root": {
			paddingTop: 12,
			paddingBottom: 12,
			".secondary": {
				display: "none",
			},
		},
	},
	role: {
		textTransform: "capitalize",
	},
	menuItem: {
		lineHeight: "140%",
		paddingTop: 12,
		paddingBottom: 12,
		whiteSpace: "normal",
		inlineSize: "250px",
	},
	menuItemSecondary: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
