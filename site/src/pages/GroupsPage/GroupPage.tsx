import { DeleteOutline as DeleteOutline, PersonAdd as PersonAdd, SettingsOutlined as SettingsOutlined } from "lucide-react";
import type { Interpolation, Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import Button from "@mui/material/Button";
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
import { Button as ShadcnButton } from "components/Button/Button";
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
import { EllipsisVertical } from "lucide-react";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useNavigate, useParams } from "react-router-dom";
import { isEveryoneGroup } from "utils/groups";
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
	const { data: permissions } = useQuery(
		groupData ? groupPermissions(groupData.id) : { enabled: false },
	);
	const addMemberMutation = useMutation(addMember(queryClient));
	const removeMemberMutation = useMutation(removeMember(queryClient));
	const deleteGroupMutation = useMutation(deleteGroup(queryClient));
	const [isDeletingGroup, setIsDeletingGroup] = useState(false);
	const isLoading = groupQuery.isLoading || !groupData || !permissions;
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;

	const helmet = (
		<Helmet>
			<title>
				{pageTitle(
					(groupData?.display_name || groupData?.name) ?? "Loading...",
				)}
			</title>
		</Helmet>
	);

	if (groupQuery.error) {
		return <ErrorAlert error={groupQuery.error} />;
	}

	if (isLoading) {
		return (
			<>
				{helmet}
				<Loader />
			</>
		);
	}
	const groupId = groupData.id;

	return (
		<>
			{helmet}

			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
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
						<Button
							component={RouterLink}
							startIcon={<SettingsOutlined />}
							to="settings"
						>
							Settings
						</Button>
						<Button
							disabled={groupData?.id === groupData?.organization_id}
							onClick={() => {
								setIsDeletingGroup(true);
							}}
							startIcon={<DeleteOutline />}
							css={styles.removeButton}
						>
							Delete&hellip;
						</Button>
					</Stack>
				)}
			</Stack>

			<Stack spacing={1}>
				{canUpdateGroup && groupData && !isEveryoneGroup(groupData) && (
					<AddGroupMember
						isLoading={addMemberMutation.isLoading}
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
			</Stack>

			{groupQuery.data && (
				<DeleteDialog
					isOpen={isDeletingGroup}
					confirmLoading={deleteGroupMutation.isLoading}
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
							<ShadcnButton
								size="icon-lg"
								variant="subtle"
								aria-label="Open menu"
							>
								<EllipsisVertical aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</ShadcnButton>
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
	removeButton: (theme) => ({
		color: theme.palette.error.main,
		"&:hover": {
			backgroundColor: "transparent",
		},
	}),
	status: {
		textTransform: "capitalize",
	},
	suspended: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default GroupPage;
