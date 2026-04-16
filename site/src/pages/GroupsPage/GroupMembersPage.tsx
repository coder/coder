import type { Interpolation, Theme } from "@emotion/react";
import { EllipsisVertical } from "lucide-react";
import { type FC, useState } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	addMember,
	groupMembersByOrganizationQueryKey,
	removeMember,
} from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import type { Group, ReducedUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { UsersFilter } from "#/components/Filter/UsersFilter";
import { LastSeen } from "#/components/LastSeen/LastSeen";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useDebouncedValue } from "#/hooks/debounce";
import { isEveryoneGroup } from "#/modules/groups";
import { AddUsersPopover } from "#/modules/users/AddUsersPopover";
import { prepareQuery } from "#/utils/filters";
import type { GroupPageOutletContext } from "./GroupPage";

const GroupMembersPage: FC = () => {
	const {
		group: groupData,
		members,
		organization,
		permissions,
		membersQuery,
		filterProps,
	} = useOutletContext<GroupPageOutletContext>();
	const queryClient = useQueryClient();
	const addMemberMutation = useMutation(addMember());
	const removeMemberMutation = useMutation(
		removeMember(queryClient, organization),
	);
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;
	const [addUsersSearch, setAddUsersSearch] = useState("");
	const debouncedSearch = useDebouncedValue(addUsersSearch, 400);

	const addableMembersQuery = useQuery({
		...organizationMembers(organization, {
			q: prepareQuery(debouncedSearch),
			limit: 25,
		}),
		select: (data) =>
			data.members.map((member) => ({
				...member,
				id: member.user_id,
			})),
		enabled:
			canUpdateGroup && Boolean(groupData) && !isEveryoneGroup(groupData),
		placeholderData: keepPreviousData,
	});

	return (
		<div className="flex flex-col w-full gap-1 pb-8">
			<div className="flex flex-row justify-between">
				<UsersFilter {...filterProps} />

				{canUpdateGroup && groupData && !isEveryoneGroup(groupData) && (
					<AddUsersPopover
						isLoading={addMemberMutation.isPending}
						// Best-effort: only excludes members on the current
						// page because the list is paginated. The server
						// rejects duplicates, so this is a UX optimization.
						existingUserIds={new Set(members.map((m) => m.id))}
						search={addUsersSearch}
						onSearchChange={setAddUsersSearch}
						usersQuery={addableMembersQuery}
						onSubmit={async (usersToAdd) => {
							const toastId = toast.loading(
								usersToAdd.length === 1
									? `Adding "${usersToAdd[0].username}" to "${groupData.name}"...`
									: `Adding ${usersToAdd.length} members to "${groupData.name}"...`,
							);

							const results = await Promise.allSettled(
								usersToAdd.map((user) =>
									addMemberMutation.mutateAsync({
										groupId: groupData.id,
										userId: user.id,
									}),
								),
							);

							const succeeded = results.filter((r) => r.status === "fulfilled");
							const failed = results.filter(
								(r): r is PromiseRejectedResult => r.status === "rejected",
							);

							// Always refresh when at least one add succeeded
							// so the member list stays in sync with the server.
							if (succeeded.length > 0) {
								await queryClient.invalidateQueries({
									queryKey: groupMembersByOrganizationQueryKey(
										organization,
										groupData.name,
									),
								});
							}

							if (failed.length > 0) {
								const msg =
									succeeded.length > 0
										? `Added ${succeeded.length} member(s), but ${failed.length} could not be added.`
										: getErrorMessage(
												failed[0].reason,
												"Failed to add members.",
											);
								toast.error(msg, {
									id: toastId,
									description: getErrorDetail(failed[0].reason),
								});
								// Throw so the popover stays open for retry.
								throw failed[0].reason;
							}

							toast.success(
								usersToAdd.length === 1
									? `Added "${usersToAdd[0].username}" to "${groupData.name}" successfully.`
									: `Added ${usersToAdd.length} members to "${groupData.name}" successfully.`,
								{ id: toastId },
							);
						}}
					/>
				)}
			</div>

			<PaginationContainer query={membersQuery} paginationUnitLabel="members">
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="w-2/5">User</TableHead>
							<TableHead className="w-3/5">Status</TableHead>
							<TableHead className="w-auto" />
						</TableRow>
					</TableHeader>

					<TableBody>
						{members.length === 0 ? (
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState message="No members found" />
								</TableCell>
							</TableRow>
						) : (
							members.map((member) => (
								<GroupMemberRow
									member={member}
									group={groupData}
									key={member.id}
									canUpdate={canUpdateGroup}
									onRemove={async () => {
										const mutation = removeMemberMutation.mutateAsync({
											groupId: groupData.id,
											userId: member.id,
										});
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
			</PaginationContainer>
		</div>
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
					subtitle={
						member.is_service_account ? "Service Account" : member.email
					}
				/>
			</TableCell>
			<TableCell
				width="40%"
				css={[styles.status, member.status === "suspended" && styles.suspended]}
			>
				<div>{member.status}</div>
				<LastSeen at={member.last_seen_at} className="text-xs" />
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
	status: {
		textTransform: "capitalize",
	},
	suspended: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default GroupMembersPage;
