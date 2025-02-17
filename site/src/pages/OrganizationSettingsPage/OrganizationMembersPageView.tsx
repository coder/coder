import PersonAdd from "@mui/icons-material/PersonAdd";
import LoadingButton from "@mui/lab/LoadingButton";
import { getErrorMessage } from "api/errors";
import type {
	Group,
	OrganizationMemberWithUserData,
	SlimRole,
	User,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { PaginationContainer } from "components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import type { PaginationResultInfo } from "hooks/usePaginatedQuery";
import { TriangleAlert, EllipsisVertical } from "lucide-react";
import { UserGroupsCell } from "pages/UsersPage/UsersTable/UserGroupsCell";
import { type FC, useState } from "react";
import { TableColumnHelpTooltip } from "./UserTable/TableColumnHelpTooltip";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
	allAvailableRoles: readonly SlimRole[] | undefined;
	canEditMembers: boolean;
	canViewMembers: boolean;
	error: unknown;
	isAddingMember: boolean;
	isUpdatingMemberRoles: boolean;
	me: User;
	members: Array<OrganizationMemberTableEntry> | undefined;
	membersQuery: PaginationResultInfo & {
		isPreviousData: boolean;
	};
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
> = ({
	allAvailableRoles,
	canEditMembers,
	canViewMembers,
	error,
	isAddingMember,
	isUpdatingMemberRoles,
	me,
	membersQuery,
	members,
	addMember,
	removeMember,
	updateMemberRoles,
}) => {
	return (
		<div>
			<SettingsHeader>
				<SettingsHeaderTitle>Members</SettingsHeaderTitle>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}

				{canEditMembers && (
					<AddOrganizationMember
						isLoading={isAddingMember}
						onSubmit={addMember}
					/>
				)}

				{!canViewMembers && (
					<div className="flex flex-row text-content-warning gap-2 items-center text-sm font-medium">
						<TriangleAlert className="size-icon-sm" />
						<p>
							You do not have permission to view members other than yourself.
						</p>
					</div>
				)}
				<PaginationContainer query={membersQuery} paginationUnitLabel="members">
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="w-2/6">User</TableHead>
								<TableHead className="w-2/6">
									<Stack direction="row" spacing={1} alignItems="center">
										<span>Roles</span>
										<TableColumnHelpTooltip variant="roles" />
									</Stack>
								</TableHead>
								<TableHead className="w-2/6">
									<Stack direction="row" spacing={1} alignItems="center">
										<span>Groups</span>
										<TableColumnHelpTooltip variant="groups" />
									</Stack>
								</TableHead>
								<TableHead className="w-auto" />
							</TableRow>
						</TableHeader>
						<TableBody>
							{members ? (
								members.map((member) => (
									<TableRow key={member.user_id} className="align-baseline">
										<TableCell>
											<AvatarData
												avatar={
													<Avatar
														fallback={member.username}
														src={member.avatar_url}
														size="lg"
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
										<UserGroupsCell userGroups={member.groups} />
										<TableCell>
											{member.user_id !== me.id && canEditMembers && (
												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button
															size="icon"
															variant="subtle"
															aria-label="Open menu"
														>
															<EllipsisVertical
																className="size-icon-md"
																aria-hidden="true"
															/>
															<span className="sr-only">Open menu</span>
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end">
														<DropdownMenuItem
															className="text-content-destructive focus:text-content-destructive"
															onClick={() => removeMember(member)}
														>
															Remove
														</DropdownMenuItem>
													</DropdownMenuContent>
												</DropdownMenu>
											)}
								</TableCell>
									</TableRow>
								))
							) : (
								<TableRow>
									<TableCell colSpan={999}>
										<Loader />
									</TableCell>
								</TableRow>
							)}
						</TableBody>
					</Table>
				</PaginationContainer>
			</div>
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
					className="w-[300px]"
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
