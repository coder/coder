import type { Interpolation, Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import MenuItem from "@mui/material/MenuItem";
import Select, { type SelectProps } from "@mui/material/Select";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type {
	Group,
	ReducedUser,
	TemplateACL,
	TemplateGroup,
	TemplateRole,
	TemplateUser,
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
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import { PersonAdd } from "lucide-react";
import { EllipsisVertical } from "lucide-react";
import { type FC, useState } from "react";
import { getGroupSubtitle } from "utils/groups";
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
			<Stack direction="row" alignItems="center" spacing={1}>
				<UserOrGroupAutocomplete
					exclude={excludeFromAutocomplete}
					templateID={templateID}
					value={selectedOption}
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
						setSelectedRole(event.target.value as TemplateRole);
					}}
				>
					<MenuItem key="use" value="use">
						Use
					</MenuItem>
					<MenuItem key="admin" value="admin">
						Admin
					</MenuItem>
				</Select>

				<LoadingButton
					loadingPosition="start"
					disabled={!selectedRole || !selectedOption}
					type="submit"
					startIcon={<PersonAdd />}
					loading={isLoading}
				>
					Add member
				</LoadingButton>
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
						Can read and use this template to create workspaces.
					</div>
				</div>
			</MenuItem>
			<MenuItem key="admin" value="admin" css={styles.menuItem}>
				<div>
					<div>Admin</div>
					<div css={styles.menuItemSecondary}>
						Can modify all aspects of this template including permissions,
						metadata, and template versions.
					</div>
				</div>
			</MenuItem>
		</Select>
	);
};

export interface TemplatePermissionsPageViewProps {
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
	const isEmpty = Boolean(
		templateACL &&
			templateACL.users.length === 0 &&
			templateACL.group.length === 0,
	);

	return (
		<>
			<PageHeader css={styles.pageHeader}>
				<PageHeaderTitle>Permissions</PageHeaderTitle>
			</PageHeader>

			<Stack spacing={2.5}>
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
				<TableContainer>
					<Table>
						<TableHead>
							<TableRow>
								<TableCell width="60%">Member</TableCell>
								<TableCell width="40%">Role</TableCell>
								<TableCell width="1%" />
							</TableRow>
						</TableHead>
						<TableBody>
							<ChooseOne>
								<Cond condition={!templateACL}>
									<TableLoader />
								</Cond>
								<Cond condition={isEmpty}>
									<TableRow>
										<TableCell colSpan={999}>
											<EmptyState
												message="No members yet"
												description="Add a member using the controls above"
											/>
										</TableCell>
									</TableRow>
								</Cond>
								<Cond>
									{templateACL?.group.map((group) => (
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
																	event.target.value as TemplateRole,
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

									{templateACL?.users.map((user) => (
										<TableRow key={user.id}>
											<TableCell>
												<AvatarData
													title={user.username}
													subtitle={user.email}
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
																	event.target.value as TemplateRole,
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
				</TableContainer>
			</Stack>
		</>
	);
};

const styles = {
	select: {
		// Match button small height
		fontSize: 14,
		width: 100,
	},

	updateSelect: {
		margin: 0,
		// Set a fixed width for the select. It avoids selects having different sizes
		// depending on how many roles they have selected.
		width: 200,

		"& .MuiSelect-root": {
			// Adjusting padding because it does not have label
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

	pageHeader: {
		paddingTop: 0,
	},
} satisfies Record<string, Interpolation<Theme>>;
