import type { Interpolation, Theme } from "@emotion/react";
import { getErrorMessage } from "api/errors";
import {
	addMember,
	deleteGroup,
	group,
	groupPermissions,
	removeMember,
} from "api/queries/groups";
import type {
	Group,
	OrganizationMemberWithUserData,
	ReducedUser,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { LastSeen } from "components/LastSeen/LastSeen";
import { Loader } from "components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
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
import {
	PaginationStatus,
	TableToolbar,
} from "components/TableToolbar/TableToolbar";
import { MemberAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import {
	EllipsisVertical,
	SettingsIcon,
	TrashIcon,
	UserPlusIcon,
} from "lucide-react";
import { isEveryoneGroup } from "modules/groups";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useNavigate, useParams } from "react-router";
import { pageTitle } from "utils/page";

const GroupPage: FC = () => {
	const { organization = "default", groupName } = useParams() as {
		organization?: string;
		groupName: string;
	};
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const groupQuery = useQuery(group(organization, groupName));
	const groupData = groupQuery.data;
	const { data: permissions } = useQuery({
		...groupPermissions(groupData?.id ?? ""),
		enabled: !!groupData,
	});
	const addMemberMutation = useMutation(addMember(queryClient));
	const removeMemberMutation = useMutation(removeMember(queryClient));
	const deleteGroupMutation = useMutation(deleteGroup(queryClient));
	const [isDeletingGroup, setIsDeletingGroup] = useState(false);
	const isLoading = groupQuery.isLoading || !groupData || !permissions;
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;

	const title = (
		<title>
			{pageTitle((groupData?.display_name || groupData?.name) ?? "Loading...")}
		</title>
	);

	if (groupQuery.error) {
		return <ErrorAlert error={groupQuery.error} />;
	}

	if (isLoading) {
		return (
			<>
				{title}
				<Loader />
			</>
		);
	}
	const groupId = groupData.id;

	return (
		<>
			{title}

			<div className="flex align-baseline justify-between w-full">
				<SettingsHeader>
					<SettingsHeaderTitle>
						{groupData?.display_name || groupData?.name || "Unknown Group"}
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage members for this group.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{canUpdateGroup && (
					<Stack direction="row" spacing={2}>
						<Button variant="outline" asChild>
							<RouterLink to="settings">
								<SettingsIcon />
								Settings
							</RouterLink>
						</Button>
						<Button
							variant="destructive"
							disabled={groupData?.id === groupData?.organization_id}
							onClick={() => {
								setIsDeletingGroup(true);
							}}
						>
							<TrashIcon />
							Delete&hellip;
						</Button>
					</Stack>
				)}
			</div>

			<div className="flex flex-col w-full gap-1">
				{canUpdateGroup && groupData && !isEveryoneGroup(groupData) && (
					<AddGroupMember
						isLoading={addMemberMutation.isPending}
						organizationId={groupData.organization_id}
						onSubmit={async (member, reset) => {
							try {
								await addMemberMutation.mutateAsync({
									groupId,
									userId: member.user_id,
								});
								reset();
								await groupQuery.refetch();
							} catch (error) {
								displayError(getErrorMessage(error, "Failed to add member."));
							}
						}}
					/>
				)}
				<TableToolbar>
					<PaginationStatus
						isLoading={Boolean(isLoading)}
						showing={groupData?.members.length ?? 0}
						total={groupData?.members.length ?? 0}
						label="members"
					/>
				</TableToolbar>

				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-2/5">User</TableHead>
							<TableHead className="w-3/5">Status</TableHead>
							<TableHead className="w-auto" />
						</TableRow>
					</TableHeader>

					<TableBody>
						{groupData?.members.length === 0 ? (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message="No members yet"
										description="Add a member using the controls above"
									/>
								</TableCell>
							</TableRow>
						) : (
							groupData?.members.map((member) => (
								<GroupMemberRow
									member={member}
									group={groupData}
									key={member.id}
									canUpdate={canUpdateGroup}
									onRemove={async () => {
										try {
											await removeMemberMutation.mutateAsync({
												groupId: groupData.id,
												userId: member.id,
											});
											await groupQuery.refetch();
											displaySuccess("Member removed successfully.");
										} catch (error) {
											displayError(
												getErrorMessage(error, "Failed to remove member."),
											);
										}
									}}
								/>
							))
						)}
					</TableBody>
				</Table>
			</div>

			{groupQuery.data && (
				<DeleteDialog
					isOpen={isDeletingGroup}
					confirmLoading={deleteGroupMutation.isPending}
					name={groupQuery.data.name}
					entity="group"
					onConfirm={async () => {
						try {
							await deleteGroupMutation.mutateAsync(groupId);
							displaySuccess("Group deleted successfully.");
							navigate("..");
						} catch (error) {
							displayError(getErrorMessage(error, "Failed to delete group."));
						}
					}}
					onCancel={() => {
						setIsDeletingGroup(false);
					}}
				/>
			)}
		</>
	);
};

interface AddGroupMemberProps {
	isLoading: boolean;
	onSubmit: (user: OrganizationMemberWithUserData, reset: () => void) => void;
	organizationId: string;
}

const AddGroupMember: FC<AddGroupMemberProps> = ({
	isLoading,
	onSubmit,
	organizationId,
}) => {
	const [selectedUser, setSelectedUser] =
		useState<OrganizationMemberWithUserData | null>(null);

	const resetValues = () => {
		setSelectedUser(null);
	};

	return (
		<form
			onSubmit={(e) => {
				e.preventDefault();

				if (selectedUser) {
					onSubmit(selectedUser, resetValues);
				}
			}}
		>
			<Stack direction="row" alignItems="center" spacing={1}>
				<MemberAutocomplete
					css={styles.autoComplete}
					value={selectedUser}
					organizationId={organizationId}
					onChange={(newValue) => {
						setSelectedUser(newValue);
					}}
				/>

				<Button disabled={!selectedUser || isLoading} type="submit">
					<Spinner loading={isLoading}>
						<UserPlusIcon className="size-icon-sm" />
					</Spinner>
					Add user
				</Button>
			</Stack>
		</form>
	);
};

interface GroupMemberRowProps {
	member: ReducedUser;
	group: Group;
	canUpdate: boolean;
	onRemove: () => void;
}

const GroupMemberRow: FC<GroupMemberRowProps> = ({
	member,
	group,
	canUpdate,
	onRemove,
}) => {
	return (
		<TableRow key={member.id}>
			<TableCell width="59%">
				<AvatarData
					avatar={
						<Avatar
							size="lg"
							fallback={member.username}
							src={member.avatar_url}
						/>
					}
					title={member.username}
					subtitle={member.email}
				/>
			</TableCell>
			<TableCell
				width="40%"
				css={[styles.status, member.status === "suspended" && styles.suspended]}
			>
				<div>{member.status}</div>
				<LastSeen at={member.last_seen_at} css={{ fontSize: 12 }} />
			</TableCell>
			<TableCell width="1%">
				{canUpdate && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button size="icon-lg" variant="subtle" aria-label="Open menu">
								<EllipsisVertical aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onClick={onRemove}
								disabled={group.id === group.organization_id}
							>
								Remove
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</TableCell>
		</TableRow>
	);
};

const styles = {
	autoComplete: {
		width: 300,
	},
	status: {
		textTransform: "capitalize",
	},
	suspended: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default GroupPage;
