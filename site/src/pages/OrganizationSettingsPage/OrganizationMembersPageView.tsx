import {
	EllipsisVerticalIcon,
	TriangleAlertIcon,
	UserPlusIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
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
	Dialog,
	DialogContent,
	DialogFooter,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import type { useFilter } from "#/components/Filter/Filter";
import { UsersFilter } from "#/components/Filter/UsersFilter";
import { Loader } from "#/components/Loader/Loader";
import { MultiUserSelect } from "#/components/MultiUserSelect/MultiUserSelect";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import type { PaginationResultInfo } from "#/hooks/usePaginatedQuery";
import { AISeatCell } from "#/modules/users/AISeatCell";
import { UserGroupsCell } from "#/pages/UsersPage/UsersTable/UserGroupsCell";
import { TableColumnHelpPopover } from "./UserTable/TableColumnHelpPopover";
import { UserRoleCell } from "./UserTable/UserRoleCell";

interface OrganizationMembersPageViewProps {
	allAvailableRoles: readonly SlimRole[] | undefined;
	canEditMembers: boolean;
	canViewMembers: boolean;
	error: unknown;
	filterProps: { filter: ReturnType<typeof useFilter> };
	isUpdatingMemberRoles: boolean;
	showAISeatColumn?: boolean;
	me: User;
	members: Array<OrganizationMemberTableEntry> | undefined;
	membersQuery: PaginationResultInfo & {
		isPlaceholderData: boolean;
	};
	addMembers: (users: User[]) => Promise<void>;
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
	filterProps,
	isUpdatingMemberRoles,
	showAISeatColumn,
	me,
	membersQuery,
	members,
	addMembers,
	removeMember,
	updateMemberRoles,
}) => {
	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Members</SettingsHeaderTitle>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				{Boolean(error) && <ErrorAlert error={error} />}

				<div className="flex flex-row justify-between">
					<UsersFilter {...filterProps} />
					{canEditMembers && <AddUsersDialog onSubmit={addMembers} />}
				</div>
				{!canViewMembers && (
					<div className="flex flex-row text-content-warning gap-2 items-center text-sm font-medium">
						<TriangleAlertIcon className="size-icon-sm" />
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
									<div className="flex flex-row items-center gap-2">
										<span>Roles</span>
										<TableColumnHelpPopover variant="roles" />
									</div>
								</TableHead>
								<TableHead className={showAISeatColumn ? "w-1/6" : "w-2/6"}>
									<div className="flex flex-row items-center gap-2">
										<span>Groups</span>
										<TableColumnHelpPopover variant="groups" />
									</div>
								</TableHead>
								{showAISeatColumn && (
									<TableHead className="w-1/6">
										<div className="flex flex-row items-center gap-2">
											<span>AI add-on</span>
											<TableColumnHelpPopover variant="ai_addon" />
										</div>
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
																<EllipsisVerticalIcon aria-hidden="true" />
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

interface AddUsersDialogProps {
	onSubmit: (users: User[]) => Promise<void>;
}

const AddUsersDialog: FC<AddUsersDialogProps> = ({ onSubmit }) => {
	const [addUserDialogOpen, setAddUserDialogOpen] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [filter, setFilter] = useState("");
	const [selected, setSelected] = useState<User[]>([]);
	const closeDialog = () => {
		setAddUserDialogOpen(false);
		setFilter("");
		setSelected([]);
	};

	return (
		<>
			<Button size="lg" onClick={() => setAddUserDialogOpen(true)}>
				<UserPlusIcon />
				Add users
			</Button>
			<Dialog
				open={addUserDialogOpen}
				onOpenChange={(open) => {
					if (!open) {
						closeDialog();
					}
				}}
			>
				<DialogContent
					data-testid="dialog"
					className="max-w-md gap-4 border-border-default bg-surface-primary p-8 text-content-primary"
				>
					<DialogTitle className="font-semibold text-content-primary">
						Add user(s)
					</DialogTitle>
					<MultiUserSelect
						filter={filter}
						setFilter={setFilter}
						onChange={(user, checked) => {
							if (checked) {
								setSelected([...selected, user]);
							} else {
								setSelected(selected.filter((s) => s.id !== user.id));
							}
						}}
						selected={selected}
					/>
					<DialogFooter className="mt-4 flex-row justify-end gap-3">
						<Button
							variant="outline"
							onClick={closeDialog}
							disabled={submitting}
						>
							Cancel
						</Button>
						<Button
							disabled={submitting || selected.length === 0}
							onClick={async () => {
								try {
									setSubmitting(true);
									await onSubmit(selected);
									closeDialog();
								} catch (error) {
									toast.error(
										getErrorMessage(error, "Failed to add members."),
										{
											description: getErrorDetail(error),
										},
									);
								} finally {
									setSubmitting(false);
								}
							}}
						>
							<Spinner loading={submitting} />
							Add users
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};
