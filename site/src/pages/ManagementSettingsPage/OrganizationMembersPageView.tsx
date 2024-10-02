import type { Interpolation, Theme } from "@emotion/react";
import PersonAdd from "@mui/icons-material/PersonAdd";
import LoadingButton from "@mui/lab/LoadingButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { getErrorMessage } from "api/errors";
import type { GroupsByUserId } from "api/queries/groups";
import type {
	Group,
	OrganizationMemberWithUserData,
	SlimRole,
	User,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarData } from "components/AvatarData/AvatarData";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import {
	MoreMenu,
	MoreMenuContent,
	MoreMenuItem,
	MoreMenuTrigger,
	ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { UserGroupsCell } from "pages/UsersPage/UsersTable/UserGroupsCell";
import { type FC, useState } from "react";
import { TableColumnHelpTooltip } from "./UserTable/TableColumnHelpTooltip";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
	allAvailableRoles: readonly SlimRole[] | undefined;
	canEditMembers: boolean;
	error: unknown;
	isAddingMember: boolean;
	isUpdatingMemberRoles: boolean;
	me: User;
	members: Array<OrganizationMemberTableEntry> | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	addMember: (user: User) => Promise<void>;
	removeMember: (member: OrganizationMemberWithUserData) => void;
	updateMemberRoles: (
		member: OrganizationMemberWithUserData,
		newRoles: string[],
	) => Promise<void>;
}

interface OrganizationMemberTableEntry extends OrganizationMemberWithUserData {
	groups: readonly Group[] | undefined;
}

export const OrganizationMembersPageView: FC<
	OrganizationMembersPageViewProps
> = (props) => {
	return (
		<div>
			<SettingsHeader title="Members" />
			<Stack>
				{Boolean(props.error) && <ErrorAlert error={props.error} />}

				{props.canEditMembers && (
					<AddOrganizationMember
						isLoading={props.isAddingMember}
						onSubmit={props.addMember}
					/>
				)}

				<TableContainer>
					<Table>
						<TableHead>
							<TableRow>
								<TableCell width="33%">User</TableCell>
								<TableCell width="33%">
									<Stack direction="row" spacing={1} alignItems="center">
										<span>Roles</span>
										<TableColumnHelpTooltip variant="roles" />
									</Stack>
								</TableCell>
								<TableCell width="33%">
									<Stack direction="row" spacing={1} alignItems="center">
										<span>Groups</span>
										<TableColumnHelpTooltip variant="groups" />
									</Stack>
								</TableCell>
								<TableCell width="1%" />
							</TableRow>
						</TableHead>
						<TableBody>
							{props.members?.map((member) => (
								<TableRow key={member.user_id}>
									<TableCell>
										<AvatarData
											avatar={
												<UserAvatar
													username={member.username}
													avatarURL={member.avatar_url}
												/>
											}
											title={member.name || member.username}
											subtitle={member.email}
										/>
									</TableCell>
									<UserRoleCell
										inheritedRoles={member.global_roles}
										roles={member.roles}
										allAvailableRoles={props.allAvailableRoles}
										oidcRoleSyncEnabled={false}
										isLoading={props.isUpdatingMemberRoles}
										canEditUsers={props.canEditMembers}
										onEditRoles={async (roles) => {
											try {
												await props.updateMemberRoles(member, roles);
												displaySuccess("Roles updated successfully.");
											} catch (error) {
												displayError(
													getErrorMessage(error, "Failed to update roles."),
												);
											}
										}}
									/>
									<UserGroupsCell userGroups={member.groups} />
									<TableCell>
										{member.user_id !== props.me.id && props.canEditMembers && (
											<MoreMenu>
												<MoreMenuTrigger>
													<ThreeDotsButton />
												</MoreMenuTrigger>
												<MoreMenuContent>
													<MoreMenuItem
														danger
														onClick={() => props.removeMember(member)}
													>
														Remove
													</MoreMenuItem>
												</MoreMenuContent>
											</MoreMenu>
										)}
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				</TableContainer>
			</Stack>
		</div>
	);
};

interface AddOrganizationMemberProps {
	isLoading: boolean;
	onSubmit: (user: User) => Promise<void>;
}

const AddOrganizationMember: FC<AddOrganizationMemberProps> = ({
	isLoading,
	onSubmit,
}) => {
	const [selectedUser, setSelectedUser] = useState<User | null>(null);

	return (
		<form
			onSubmit={async (event) => {
				event.preventDefault();

				if (selectedUser) {
					try {
						await onSubmit(selectedUser);
						setSelectedUser(null);
					} catch (error) {
						displayError(getErrorMessage(error, "Failed to add member."));
					}
				}
			}}
		>
			<Stack direction="row" alignItems="center" spacing={1}>
				<UserAutocomplete
					css={styles.autoComplete}
					value={selectedUser}
					onChange={(newValue) => {
						setSelectedUser(newValue);
					}}
				/>

				<LoadingButton
					loadingPosition="start"
					disabled={!selectedUser}
					type="submit"
					startIcon={<PersonAdd />}
					loading={isLoading}
				>
					Add user
				</LoadingButton>
			</Stack>
		</form>
	);
};

const styles = {
	role: (theme) => ({
		backgroundColor: theme.roles.notice.background,
		borderColor: theme.roles.notice.outline,
	}),
	globalRole: (theme) => ({
		backgroundColor: theme.roles.inactive.background,
		borderColor: theme.roles.inactive.outline,
	}),
	autoComplete: {
		width: 300,
	},
} satisfies Record<string, Interpolation<Theme>>;
