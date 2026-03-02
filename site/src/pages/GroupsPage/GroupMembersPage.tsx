import type { Interpolation, Theme } from "@emotion/react";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { addMember, removeMember } from "api/queries/groups";
import type {
	Group,
	OrganizationMemberWithUserData,
	ReducedUser,
} from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { EmptyState } from "components/EmptyState/EmptyState";
import { LastSeen } from "components/LastSeen/LastSeen";
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
import { EllipsisVertical, UserPlusIcon } from "lucide-react";
import { isEveryoneGroup } from "modules/groups";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import type { GroupPageOutletContext } from "./GroupPage";

const GroupMembersPage: FC = () => {
	const {
		group: groupData,
		organization,
		permissions,
		groupQuery,
	} = useOutletContext<GroupPageOutletContext>();
	const queryClient = useQueryClient();
	const addMemberMutation = useMutation(addMember(queryClient, organization));
	const removeMemberMutation = useMutation(
		removeMember(queryClient, organization),
	);
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;
	const groupId = groupData.id;

	return (
		<div className="flex flex-col w-full gap-1 pb-8">
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
							toast.error(getErrorMessage(error, "Failed to add member."), {
								description: getErrorDetail(error),
							});
						}
					}}
				/>
			)}
			<TableToolbar>
				<PaginationStatus
					isLoading={false}
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
									const mutation = removeMemberMutation.mutateAsync(
										{
											groupId: groupData.id,
											userId: member.id,
										},
										{
											onSuccess: () => {
												groupQuery.refetch();
											},
										},
									);
									toast.promise(mutation, {
										loading: `Removing member "${member.username}" from "${groupData.name}"...`,
										success: `Member "${member.username}" has been removed from "${groupData.name}" successfully.`,
										error: (error) => ({
											message: `Failed to remove member "${member.username}" from "${groupData.name}".`,
											description: getErrorDetail(error),
										}),
									});
								}}
							/>
						))
					)}
				</TableBody>
			</Table>
		</div>
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

export default GroupMembersPage;
