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
	Organization,
	OrganizationMemberWithUserData,
	SlimRole,
	User,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { AvatarData } from "components/AvatarData/AvatarData";
import { Breadcrumbs, Crumb } from "components/Breadcrumbs/Breadcrumbs";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import {
	MoreMenu,
	MoreMenuContent,
	MoreMenuItem,
	MoreMenuTrigger,
	ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";
import { Stack } from "components/Stack/Stack";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type FC, useState } from "react";
import { TableColumnHelpTooltip } from "./UserTable/TableColumnHelpTooltip";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
	allAvailableRoles: readonly SlimRole[] | undefined;
	canEditMembers: boolean;
	error: unknown;
	isAddingMember: boolean;
	isUpdatingMemberRoles: boolean;
	organization: Organization;
	me: User;
	members: Array<OrganizationMemberTableEntry> | undefined;
	groupsByUserId: GroupsByUserId | undefined;
	addMember: (user: User) => Promise<void>;
	removeMember: (member: OrganizationMemberWithUserData) => Promise<void>;
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
> = ({
	allAvailableRoles,
	canEditMembers,
	error,
	isAddingMember,
	isUpdatingMemberRoles,
	organization,
	me,
	members,
	addMember,
	removeMember,
	updateMemberRoles,
}) => {
	return (
		<Stack>
			<Breadcrumbs>
				<Crumb>Organizations</Crumb>
				<Crumb href={`/organizations/${organization}`}>
					{organization.display_name || organization.name}
				</Crumb>
				<Crumb href={`/organizations/${organization}/members`} active>
					Members
				</Crumb>
			</Breadcrumbs>

			{Boolean(error) && <ErrorAlert error={error} />}

			{canEditMembers && (
				<AddOrganizationMember
					isLoading={isAddingMember}
					onSubmit={addMember}
				/>
			)}

			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="50%">User</TableCell>
							<TableCell width="49%">
								<Stack direction="row" spacing={1} alignItems="center">
									<span>Roles</span>
									<TableColumnHelpTooltip variant="roles" />
								</Stack>
							</TableCell>
							<TableCell width="1%"></TableCell>
						</TableRow>
					</TableHead>
					<TableBody>
						{members?.map((member) => (
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
									allAvailableRoles={allAvailableRoles}
									oidcRoleSyncEnabled={false}
									isLoading={isUpdatingMemberRoles}
									canEditUsers={canEditMembers}
									onEditRoles={async (roles) => {
										try {
											await updateMemberRoles(member, roles);
											displaySuccess("Roles updated successfully.");
										} catch (error) {
											displayError(
												getErrorMessage(error, "Failed to update roles."),
											);
										}
									}}
								/>
								<TableCell>
									{member.user_id !== me.id && canEditMembers && (
										<MoreMenu>
											<MoreMenuTrigger>
												<ThreeDotsButton />
											</MoreMenuTrigger>
											<MoreMenuContent>
												<MoreMenuItem
													danger
													onClick={async () => {
														try {
															await removeMember(member);
															displaySuccess("Member removed successfully.");
														} catch (error) {
															displayError(
																getErrorMessage(
																	error,
																	"Failed to remove member.",
																),
															);
														}
													}}
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
