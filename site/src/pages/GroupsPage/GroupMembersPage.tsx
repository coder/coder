import dayjs from "dayjs";
import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import type { GroupMemberWithAICostControl } from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { addMembers, groupAIBudget, removeMember } from "#/api/queries/groups";
import { meAISpend } from "#/api/queries/users";
import type {
	Group,
	OrganizationMemberWithUserData,
} from "#/api/typesGenerated";
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
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { UsersFilter } from "#/components/Filter/UsersFilter";
import { LastSeen } from "#/components/LastSeen/LastSeen";
import { MultiMemberSelect } from "#/components/MultiUserSelect/MultiUserSelect";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { isEveryoneGroup } from "#/modules/groups";
import { cn } from "#/utils/cn";
import { formatBudgetUSD } from "#/utils/currency";
import {
	GroupMemberBudgetCells,
	isBudgetFromOtherGroup,
} from "./GroupMemberBudgetCells";
import type { GroupPageOutletContext } from "./GroupPage";
import { InfoIconTooltip } from "./InfoIconTooltip";
import { UserAIBudgetOverrideDialog } from "./UserAIBudgetOverrideDialog";

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
	const addMembersMutation = useMutation(addMembers(queryClient, organization));
	const removeMemberMutation = useMutation(
		removeMember(queryClient, organization),
	);
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;
	const [budgetUser, setBudgetUser] =
		useState<GroupMemberWithAICostControl | null>(null);

	const { experiments } = useDashboard();
	// TODO(AIGOV-443): drop the experiment gate once cost control is stable.
	const aibridgeVisible =
		useFeatureVisibility().aibridge &&
		experiments.includes("ai-gateway-cost-control");
	// Spend resets at period_end, rendered in the viewer's local time.
	const { data: aiSpend } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});
	const { data: groupBudget } = useQuery({
		...groupAIBudget(groupData.id),
		enabled: aibridgeVisible,
	});
	const resetAt = aiSpend
		? dayjs(aiSpend.period_end).format("MMM D, YYYY h:mm A")
		: undefined;
	let aiBudgetNote = "Monthly AI spend for this user.";
	if (resetAt) {
		aiBudgetNote += ` Resets ${resetAt}.`;
	}
	if (groupBudget && groupBudget.spend_limit_micros > 0) {
		aiBudgetNote += ` The group's default limit is ${formatBudgetUSD(
			groupBudget.spend_limit_micros,
		)} per member.`;
	}

	return (
		<div className="flex flex-col w-full gap-1 pb-8">
			<div className="flex flex-row justify-between">
				<UsersFilter {...filterProps} />

				{canUpdateGroup && groupData && !isEveryoneGroup(groupData) && (
					<AddUsersDialog
						organizationId={groupData.organization_id}
						onSubmit={async (users) => {
							await addMembersMutation.mutateAsync({
								groupId: groupData.id,
								userIds: users.map((u) => u.user_id),
							});
						}}
					/>
				)}
			</div>

			<PaginationContainer query={membersQuery} paginationUnitLabel="members">
				<Table aria-label="Group members">
					<TableHeader>
						<TableRow>
							<TableHead className={aibridgeVisible ? undefined : "w-2/5"}>
								User
							</TableHead>
							<TableHead className={aibridgeVisible ? undefined : "w-3/5"}>
								Status
							</TableHead>
							{aibridgeVisible && (
								<>
									<TableHead>
										<div className="flex items-center gap-1">
											AI budget
											<InfoIconTooltip message={aiBudgetNote} />
										</div>
									</TableHead>
									<TableHead>
										<div className="flex items-center gap-1">
											Budget group
											<InfoIconTooltip message="The group or individual budget currently responsible for this user's AI spend. Admins can reassign this at any time, so spend history may span multiple sources." />
										</div>
									</TableHead>
								</>
							)}
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
									showAIBudget={aibridgeVisible}
									onManageAIBudget={() => setBudgetUser(member)}
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

			{aibridgeVisible && budgetUser && (
				<UserAIBudgetOverrideDialog
					open
					onOpenChange={(open) => {
						if (!open) {
							setBudgetUser(null);
						}
					}}
					user={budgetUser}
					currentGroup={groupData}
					effectiveGroupId={budgetUser.ai_cost_control?.effective_group_id}
				/>
			)}
		</div>
	);
};

interface AddUsersDialogProps {
	onSubmit: (users: OrganizationMemberWithUserData[]) => Promise<void>;
	organizationId: string;
}

const AddUsersDialog: FC<AddUsersDialogProps> = ({
	onSubmit,
	organizationId,
}) => {
	const [addUserDialogOpen, setAddUserDialogOpen] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [filter, setFilter] = useState("");
	const [selected, setSelected] = useState<OrganizationMemberWithUserData[]>(
		[],
	);
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
					<MultiMemberSelect
						organizationId={organizationId}
						filter={filter}
						setFilter={setFilter}
						onChange={(user, checked) => {
							if (checked) {
								setSelected([...selected, user]);
							} else {
								setSelected(selected.filter((s) => s.user_id !== user.user_id));
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

interface GroupMemberRowProps {
	member: GroupMemberWithAICostControl;
	group: Group;
	canUpdate: boolean;
	showAIBudget: boolean;
	onManageAIBudget: () => void;
	onRemove: () => void;
}

const GroupMemberRow: FC<GroupMemberRowProps> = ({
	member,
	group,
	canUpdate,
	showAIBudget,
	onManageAIBudget,
	onRemove,
}) => {
	const costControl = member.ai_cost_control;
	const budgetFromOtherGroup = isBudgetFromOtherGroup(costControl, group);

	return (
		<TableRow key={member.id}>
			<TableCell width={showAIBudget ? undefined : "59%"}>
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
				width={showAIBudget ? undefined : "40%"}
				className={cn(
					"capitalize",
					member.status === "suspended" ? "text-content-secondary" : "",
				)}
			>
				<div>{member.status}</div>
				<LastSeen at={member.last_seen_at} className="text-xs" />
			</TableCell>
			{showAIBudget && (
				<GroupMemberBudgetCells
					group={group}
					userID={member.id}
					costControl={costControl}
				/>
			)}
			<TableCell className="w-1 whitespace-nowrap">
				{canUpdate && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button size="icon-lg" variant="subtle" aria-label="Open menu">
								<EllipsisVerticalIcon aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							{showAIBudget && (
								<DropdownMenuItem
									onClick={onManageAIBudget}
									disabled={budgetFromOtherGroup}
								>
									Manage AI budget
								</DropdownMenuItem>
							)}
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

export default GroupMembersPage;
