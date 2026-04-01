import { EllipsisVertical, TriangleAlert } from "lucide-react";
import { type ComponentProps, type FC, useState } from "react";
import { useQuery } from "react-query";
import { toast } from "sonner";
import { users } from "#/api/queries/users";
import type {
	Group,
	OrganizationMemberWithUserData,
	SlimRole,
	User,
} from "#/api/typesGenerated";
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
import { UsersFilter } from "#/components/Filter/UsersFilter";
import { Loader } from "#/components/Loader/Loader";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Stack } from "#/components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useDebouncedValue } from "#/hooks/debounce";
import type { PaginationResultInfo } from "#/hooks/usePaginatedQuery";
import {
	type AddableUser,
	AddUsersPopover,
} from "#/modules/users/AddUsersPopover";
import { AISeatCell } from "#/modules/users/AISeatCell";
import { UserGroupsCell } from "#/pages/UsersPage/UsersTable/UserGroupsCell";
import { prepareQuery } from "#/utils/filters";
import { TableColumnHelpPopover } from "./UserTable/TableColumnHelpPopover";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
	allAvailableRoles: readonly SlimRole[] | undefined;
	canEditMembers: boolean;
	canViewMembers: boolean;
	filterProps: ComponentProps<typeof UsersFilter>;
	error: unknown;
	isAddingMember: boolean;
	isUpdatingMemberRoles: boolean;
	showAISeatColumn?: boolean;
	me: User;
	members: Array<OrganizationMemberTableEntry> | undefined;
	membersQuery: PaginationResultInfo & {
		isPlaceholderData: boolean;
	};
	addMembers: (users: readonly AddableUser[]) => Promise<void>;
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
	filterProps,
	error,
	isAddingMember,
	isUpdatingMemberRoles,
	showAISeatColumn,
	me,
	membersQuery,
	members,
	addMembers,
	removeMember,
	updateMemberRoles,
}) => {
	const [addUsersSearch, setAddUsersSearch] = useState("");
	const debouncedSearch = useDebouncedValue(addUsersSearch, 400);
	const addableUsersQuery = useQuery({
		...users({
			q: prepareQuery(debouncedSearch),
			limit: 50,
		}),
		select: (data) => data.users,
		enabled: canEditMembers,
	});

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Members</SettingsHeaderTitle>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}

				<div className="flex flex-row flex-wrap items-start justify-between gap-4">
					<UsersFilter {...filterProps} />
					{canEditMembers && (
						<AddUsersPopover
							isLoading={isAddingMember}
							onSubmit={addMembers}
							existingUserIds={new Set(members?.map((m) => m.user_id) ?? [])}
							search={addUsersSearch}
							onSearchChange={setAddUsersSearch}
							usersQuery={addableUsersQuery}
						/>
					)}
				</div>

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
										<TableColumnHelpPopover variant="roles" />
									</Stack>
								</TableHead>
								<TableHead className={showAISeatColumn ? "w-1/6" : "w-2/6"}>
									<Stack direction="row" spacing={1} alignItems="center">
										<span>Groups</span>
										<TableColumnHelpPopover variant="groups" />
									</Stack>
								</TableHead>
								{showAISeatColumn && (
									<TableHead className="w-1/6">
										<Stack direction="row" spacing={1} alignItems="center">
											<span>AI add-on</span>
											<TableColumnHelpPopover variant="ai_addon" />
										</Stack>
									</TableHead>
								)}
								<TableHead className="w-px whitespace-nowrap text-right" />
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
												// React doesn't mind uncaught errors in event handlers,
												// but testing-library does.
												try {
													await updateMemberRoles(member, roles);
													toast.success(
														`Roles of "${member.username}" updated successfully.`,
													);
												} catch {}
											}}
										/>
										<UserGroupsCell userGroups={member.groups} />
										{showAISeatColumn && (
											<AISeatCell hasAISeat={member.has_ai_seat} />
										)}
										<TableCell className="w-px whitespace-nowrap text-right">
											<div className="flex justify-end">
												{member.user_id !== me.id && canEditMembers && (
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
																onClick={() => removeMember(member)}
															>
																Remove
															</DropdownMenuItem>
														</DropdownMenuContent>
													</DropdownMenu>
												)}
											</div>
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
