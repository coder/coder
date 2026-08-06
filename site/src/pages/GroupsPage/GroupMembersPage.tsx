import dayjs from "dayjs";
import { EllipsisVerticalIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	groupAIBudget,
	groupMembersAISpend,
	removeMember,
} from "#/api/queries/groups";
import { meAISpend } from "#/api/queries/users";
import type {
	Group,
	GroupMemberAISpend,
	ReducedUser,
} from "#/api/typesGenerated";
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
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { cn } from "#/utils/cn";
import { formatBudgetUSD } from "#/utils/currency";
import { SpendEstimateDocsLink } from "./AICostControl";
import {
	effectiveBudgetGroup,
	GroupMemberBudgetCells,
} from "./GroupMemberBudgetCells";
import type { GroupPageOutletContext } from "./GroupPage";
import { StatusIconTooltip } from "./StatusIconTooltip";
import { UserAIBudgetOverrideDialog } from "./UserAIBudgetOverrideDialog";

type MemberWithSpend = ReducedUser & {
	readonly spend: GroupMemberAISpend | undefined;
};

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
	const removeMemberMutation = useMutation(
		removeMember(queryClient, organization),
	);
	const { permissions: sitePermissions } = useAuthenticated();
	const canUpdateGroup = permissions ? permissions.canUpdateGroup : false;
	// Setting a user's AI budget override updates both the user and the group
	// its spend is charged to, so it needs permission on both.
	const canUpdateBudgetOverride = canUpdateGroup && sitePermissions.updateUsers;
	const [budgetUser, setBudgetUser] = useState<MemberWithSpend | null>(null);

	const aibridgeVisible = Boolean(useFeatureVisibility().aibridge);
	const { data: aiSpend } = useQuery({
		...meAISpend(),
		enabled: aibridgeVisible,
	});
	const { data: groupBudget } = useQuery({
		...groupAIBudget(groupData.id),
		enabled: aibridgeVisible,
	});
	const memberIds = members.map((member) => member.id);
	const membersSpendQuery = useQuery({
		...groupMembersAISpend(groupData.id, memberIds),
		enabled: aibridgeVisible && memberIds.length > 0,
	});
	const spendByUserId = new Map(
		membersSpendQuery.data?.members.map((spend) => [spend.user_id, spend]) ??
			[],
	);
	// Join each member with its spend (undefined when loading, failed, or
	// omitted by the backend) so each row gets a single object.
	const membersWithSpend = members.map(
		(member): MemberWithSpend => ({
			...member,
			spend: spendByUserId.get(member.id),
		}),
	);
	const aiBudgetNote = [
		"Estimated monthly AI spend for this user.",
		// Spend resets at period_end, rendered in the viewer's local time.
		aiSpend &&
			`Resets ${dayjs(aiSpend.period_end).format("MMM D, YYYY h:mm A")}.`,
		// A $0 default still shows: it means no spending allowance.
		groupBudget &&
			`The group's default limit is ${formatBudgetUSD(groupBudget.spend_limit_micros)} per member.`,
	]
		.filter(Boolean)
		.join(" ");

	useEffect(() => {
		if (membersSpendQuery.error) {
			toast.error(
				getErrorMessage(membersSpendQuery.error, "Unable to load AI spend."),
				{
					description: getErrorDetail(membersSpendQuery.error),
				},
			);
		}
	}, [membersSpendQuery.error]);

	return (
		<div className="flex flex-col w-full gap-1 pb-8">
			<UsersFilter {...filterProps} />

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
											{membersSpendQuery.isError ? (
												<StatusIconTooltip
													kind="warning"
													message="AI spend couldn't be loaded, so budgets aren't shown."
												/>
											) : (
												<StatusIconTooltip
													message={
														<>
															{aiBudgetNote} <SpendEstimateDocsLink />
														</>
													}
												/>
											)}
										</div>
									</TableHead>
									<TableHead>
										<div className="flex items-center gap-1">
											Budget group
											<StatusIconTooltip message="The group or individual budget currently responsible for this user's AI spend. Admins can reassign this at any time, so spend history may span multiple sources." />
										</div>
									</TableHead>
								</>
							)}
							<TableHead className="w-auto" />
						</TableRow>
					</TableHeader>

					<TableBody>
						{members.length === 0 ? (
							<TableEmpty message="No members found" />
						) : (
							membersWithSpend.map((member) => (
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
					effectiveGroupId={budgetUser.spend?.effective_group_id}
					canUpdate={canUpdateBudgetOverride}
				/>
			)}
		</div>
	);
};

interface GroupMemberRowProps {
	member: MemberWithSpend;
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
	const budgetFromOtherGroup =
		effectiveBudgetGroup(member.spend, group).kind === "other";

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
					spend={member.spend}
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
