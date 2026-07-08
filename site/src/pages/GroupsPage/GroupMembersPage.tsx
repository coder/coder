import dayjs from "dayjs";
import { EllipsisVerticalIcon, UserPlusIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import type {
	GroupMemberAICostControl,
	GroupMemberWithAICostControl,
} from "#/api/api";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { addMembers, groupById, removeMember } from "#/api/queries/groups";
import { meAISpend } from "#/api/queries/users";
import type {
	Group,
	OrganizationMemberWithUserData,
} from "#/api/typesGenerated";
import { AIBudgetUsage } from "#/components/AIBudgetUsage/AIBudgetUsage";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
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
	// TODO(AIGOV-443): remove the ai-gateway-cost-control experiment gate once
	// the cost-control feature is stable.
	const aibridgeVisible =
		Boolean(useFeatureVisibility().aibridge) &&
		experiments.includes("ai-gateway-cost-control");

	// Spend resets at period_end, rendered in the viewer's local time.
	const { data: aiSpend } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});
	const resetAt = aiSpend
		? dayjs(aiSpend.period_end).format("MMM D, YYYY h:mm A")
		: undefined;
	const aiBudgetNote = `Monthly AI API cost for this user.${
		resetAt ? ` Resets ${resetAt}` : ""
	}`;

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
											Budget source
											<InfoIconTooltip
												message={
													<>
														Users with group type will inherit the group budget
														allowance.
														<br />
														<br />
														Users with individual type have a budget override.
													</>
												}
											/>
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
	// A budget from another group is managed there, not here.
	const budgetFromOtherGroup =
		costControl?.effective_group_id != null &&
		costControl.effective_group_id !== group.id;

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
				<GroupMemberAIBudgetCells
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
									AI Budget
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

// Escaped em dash for empty cells; a literal one trips the emdash lint check.
const emDash = "\u2014";

const GroupMemberAIBudgetCells: FC<{
	group: Group;
	userID: string;
	costControl: GroupMemberAICostControl | undefined;
}> = ({ group, userID, costControl }) => {
	// Budget and source apply only on the member's effective group.
	const onEffectiveGroup = costControl?.effective_group_id === group.id;

	let budget: ReactNode = emDash;
	let source: ReactNode = emDash;
	if (costControl && onEffectiveGroup) {
		budget = (
			<AIBudgetUsage
				currentSpend={costControl.current_spend_micros}
				spendLimit={costControl.spend_limit_micros}
			/>
		);
		if (costControl.limit_source) {
			source = (
				<Badge variant="default" size="sm">
					{budgetTypeLabels[costControl.limit_source]}
				</Badge>
			);
		}
	} else if (costControl) {
		// Another group governs the budget; name it in a tooltip.
		const attribution = (
			<span className="inline-flex items-center gap-1 text-content-disabled">
				{emDash}
				<MemberBudgetSourceTooltip groupId={costControl.effective_group_id} />
			</span>
		);
		budget = attribution;
		source = attribution;
	}

	return (
		<>
			<TableCell
				data-testid={`member-ai-budget-${userID}`}
				className="whitespace-nowrap tabular-nums"
			>
				{budget}
			</TableCell>
			<TableCell>{source}</TableCell>
		</>
	);
};

// Names the group whose budget governs a member, resolving the id to a name.
const MemberBudgetSourceTooltip: FC<{ groupId: string | null }> = ({
	groupId,
}) => {
	const { data: group } = useQuery({
		...groupById(groupId ?? "", { exclude_members: true }),
		enabled: Boolean(groupId),
	});
	const name = group?.display_name || group?.name;
	return (
		<InfoIconTooltip
			className="text-content-disabled"
			message={
				name ? (
					<>
						This user's budget is attributed to the{" "}
						<span className="font-medium text-content-primary">{name}</span>{" "}
						group.
					</>
				) : (
					"This user's budget is attributed to another group."
				)
			}
		/>
	);
};

const budgetTypeLabels: Record<
	NonNullable<GroupMemberAICostControl["limit_source"]>,
	string
> = {
	group: "Group",
	override: "Individual",
};

export default GroupMembersPage;
