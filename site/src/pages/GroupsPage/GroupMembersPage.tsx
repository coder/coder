import type { Interpolation, Theme } from "@emotion/react";
import { EllipsisVerticalIcon, InfoIcon, UserPlusIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useOutletContext } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { addMembers, removeMember } from "#/api/queries/groups";
import type {
	Group,
	OrganizationMemberWithUserData,
	ReducedUser,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
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
import { Input } from "#/components/Input/Input";
import { LastSeen } from "#/components/LastSeen/LastSeen";
import { MultiMemberSelect } from "#/components/MultiUserSelect/MultiUserSelect";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { isEveryoneGroup } from "#/modules/groups";
import { cn } from "#/utils/cn";
import type { GroupPageOutletContext } from "./GroupPage";

// Budget information for an individual group member.
export type MemberBudgetInfo = {
	spentUSD: number;
	limitUSD: number;
	// "group" means the budget is attributed to a group;
	// "individual" means the member has a personal budget.
	budgetType: "group" | "individual";
	// The group name the budget is attributed to, when budgetType
	// is "group".
	attributedGroup?: string;
	// Custom monthly budget override for this member, if any.
	customMonthlyBudget?: number;
	// Breakdown of spending across groups.
	breakdown?: BudgetBreakdownItem[];
};

export type BudgetBreakdownItem = {
	groupName: string;
	label: string;
	avatarUrl?: string;
	amountUSD: number;
	dateRange?: string;
	selected?: boolean;
};

export type MemberRole = {
	name: string;
};

// Extended member info for the cost controls mockup.
export type MemberWithBudget = {
	member: ReducedUser;
	budget?: MemberBudgetInfo;
	roles?: MemberRole[];
};

// Seed-based random number so values stay stable across re-renders.
function seededRandom(seed: string): number {
	let h = 0;
	for (let i = 0; i < seed.length; i++) {
		h = Math.imul(31, h) + seed.charCodeAt(i);
	}
	return ((h >>> 0) % 1000) / 1000;
}

const MOCK_ROLE_SETS: MemberRole[][] = [
	[{ name: "Admin" }, { name: "Elite AI" }],
	[{ name: "Member" }, { name: "AI" }],
	[{ name: "Member" }],
	[{ name: "Service account" }],
];

const MOCK_GROUPS = ["Pineapple", "DevOps", "Flaming devs"];

// Generate plausible mock budget data from a member ID.
function mockBudgetForMember(
	memberId: string,
	index: number,
	totalMembers: number,
): { budget?: MemberBudgetInfo; roles: MemberRole[] } {
	const r = seededRandom(memberId);
	const roles = MOCK_ROLE_SETS[index % MOCK_ROLE_SETS.length];

	// First member gets no budget (like the "Username" row in the
	// mockup screenshot).
	if (index === 0) {
		return { roles };
	}

	// Last member is always over budget so we get a red ring example.
	const isForceOverBudget = index === totalMembers - 1 && totalMembers > 2;

	const limitChoices = [7000, 9000, 12000, 15000];
	const limit = isForceOverBudget
		? 7000
		: limitChoices[Math.floor(r * limitChoices.length)];
	const spent = isForceOverBudget
		? 6978
		: Math.round(limit * (0.5 + r * 0.49));
	const isGroup = index % 2 === 1;
	const groupName = MOCK_GROUPS[Math.floor(r * MOCK_GROUPS.length)];

	const budget: MemberBudgetInfo = {
		spentUSD: spent,
		limitUSD: limit,
		budgetType: isGroup ? "group" : "individual",
		attributedGroup: "DevOps",
		customMonthlyBudget: isGroup ? undefined : limit,
		breakdown: !isGroup
			? [
				{
					groupName: "DevOps",
					label: "individual override",
					avatarUrl: "/emojis/1f34b.png",
					amountUSD: Math.round(spent * 0.06),
					dateRange: "May 17-Today",
					selected: true,
				},
				{
					groupName: "DevOps",
					label: "group",
					avatarUrl: "/emojis/1f34b.png",
					amountUSD: Math.round(spent * 0.75),
					dateRange: "May 8-16",
				},
				{
					groupName: "Flaming devs",
					label: "group",
					avatarUrl: "/emojis/1f525.png",
					amountUSD: Math.round(spent * 0.19),
					dateRange: "May 1-7",
				},
			]
			: undefined,
	};

	return { budget, roles };
}

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

	// Build mock budget data from real members.
	const membersWithBudget: MemberWithBudget[] = useMemo(
		() =>
			members.map((member, i) => {
				const mock = mockBudgetForMember(member.id, i, members.length);
				const m = member as ReducedUser;
				// Override status for the first member so the
				// presentation shows a dormant example.
				const patched =
					i === 0 ? { ...m, status: "dormant" as const } : m;
				return { member: patched, ...mock };
			}),
		[members],
	);

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
				<GroupMembersMockupTable membersWithBudget={membersWithBudget} />
			</PaginationContainer>
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
				Add user
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

// Cost controls mockup components below.

const InfoTooltip: FC<{ text: string }> = ({ text }) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<InfoIcon className="size-icon-xs text-content-secondary cursor-default" />
		</TooltipTrigger>
		<TooltipContent align="end" className="whitespace-pre-line max-w-[300px]">{text}</TooltipContent>
	</Tooltip>
);

const BUDGET_WARNING_THRESHOLD = 0.90;

const formatUSD = (amount: number): string =>
	`$${amount.toLocaleString("en-US")}`;

// The "AI spend limit" dialog for updating a member's budget.
export const AISpendLimitDialog: FC<{
	open: boolean;
	onOpenChange: (open: boolean) => void;
	memberName: string;
	memberEmail: string;
	memberAvatarUrl?: string;
	budget: number;
	hasCustomBudget: boolean;
	assignedGroup: string;
	groups: string[];
}> = ({
	open,
	onOpenChange,
	memberName,
	memberEmail,
	memberAvatarUrl,
	budget,
	hasCustomBudget,
	assignedGroup,
	groups,
}) => (
	<Dialog open={open} onOpenChange={onOpenChange}>
		<DialogContent className="max-w-lg gap-0 border-border-default bg-surface-primary p-0 text-content-primary w-full">
			<div className="flex flex-col gap-4 p-8 pb-6">
				<DialogTitle className="text-xl font-semibold text-content-primary">
					AI spend limit
				</DialogTitle>
				<AvatarData
					avatar={
						<Avatar
							size="lg"
								fallback={memberName}
								src={memberAvatarUrl}
							/>
						}
						title={memberName}
						subtitle={memberEmail}
					/>
					<DialogDescription className="text-sm text-content-secondary">
						{memberName} has {hasCustomBudget ? "a custom" : "the default"} {formatUSD(budget)} USD / month budget.
						{" "}Their budget is assigned to{" "}
						<span className="font-semibold text-content-primary">
							{assignedGroup}
						</span>
						.
				</DialogDescription>
				</div>

			<div className="border-0 border-t border-solid border-border" />

			<div className="flex flex-col gap-1 px-8 pt-6">
				<label className="text-sm font-semibold text-content-primary">
					Custom monthly budget
				</label>
				<span className="text-xs text-content-secondary">
					Member will use this rate instead of the group default.
				</span>
				<div className="relative mt-1">
					<span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-content-disabled">
						$
					</span>
					<Input
						defaultValue={hasCustomBudget ? budget.toLocaleString("en-US") : ""}
						placeholder={!hasCustomBudget ? budget.toLocaleString("en-US") : ""}
						className="pl-7 pr-14 placeholder:text-content-disabled"
					/>
					<span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-content-disabled">
						USD
					</span>
				</div>
			</div>

			<div className="flex flex-col gap-2 px-8 pt-6">
				<label className="text-sm font-semibold text-content-primary">
					Budget assigned to
				</label>
				<Select defaultValue={assignedGroup}>
					<SelectTrigger>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						{groups.map((g) => (
							<SelectItem key={g} value={g}>
								{g}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>

			<DialogFooter className="flex-row justify-end gap-3 px-8 py-6">
				<Button variant="outline" onClick={() => onOpenChange(false)}>
					Cancel
				</Button>
				<Button onClick={() => onOpenChange(false)}>Update</Button>
			</DialogFooter>
		</DialogContent>
	</Dialog>
);

// Tooltip showing per-group spend breakdown for a member.
const BudgetBreakdownTooltip: FC<{
	memberName: string;
	groupName: string;
	breakdown: BudgetBreakdownItem[];
	children: React.ReactNode;
}> = ({ memberName, groupName, breakdown, children }) => {
	const totalSpent = breakdown.reduce((sum, b) => sum + b.amountUSD, 0);

	return (
		<Tooltip>
			<TooltipTrigger asChild>{children}</TooltipTrigger>
			<TooltipContent
				align="end"
				className="w-[28rem] p-5"
			>
				<p className="text-sm text-content-secondary pb-4 m-0">
					<span className="font-semibold text-content-primary">
						{memberName}
					</span>{" "}
					has spent <span className="font-semibold text-content-primary">{formatUSD(totalSpent)}</span> USD this month across {new Set(breakdown.map((b) => b.groupName)).size} groups.
				</p>
				<div className="flex flex-col gap-2">
					{breakdown.map((item) => (
							<div
								key={`${item.groupName}-${item.label}`}
								className={cn(
									"grid grid-cols-[auto_1fr_5.5rem_4rem] items-center gap-x-2.5 rounded-lg px-3 py-2.5",
									item.selected && "bg-surface-secondary",
								)}
							>
								<Avatar
									size="lg"
									variant="icon"
									fallback={item.groupName}
									src={item.avatarUrl}
								/>
								<div className="flex flex-col">
									<span className="text-sm text-content-primary">
										{item.groupName}
									</span>
									<span className="text-xs text-content-secondary">
										({item.label})
									</span>
								</div>
								<span className="text-xs text-content-secondary whitespace-nowrap">
									{item.dateRange ?? ""}
								</span>
								<span className="text-sm tabular-nums text-content-primary font-medium text-right">
									{formatUSD(item.amountUSD)}
								</span>
							</div>
					))}
				</div>
			</TooltipContent>
		</Tooltip>
	);
};

// The full mockup table for the cost controls presentation.
export const GroupMembersMockupTable: FC<{
	membersWithBudget: MemberWithBudget[];
}> = ({ membersWithBudget }) => {
	const [spendLimitOpen, setSpendLimitOpen] = useState(false);
	const [selectedMember, setSelectedMember] =
		useState<MemberWithBudget | null>(null);

	return (
		<>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>User</TableHead>
						<TableHead>Roles</TableHead>
						<TableHead>Status</TableHead>
						<TableHead>
							<span className="inline-flex items-center gap-1">
								AI budget
								<InfoTooltip text="Total monthly budget attributed to this group" />
							</span>
						</TableHead>
						<TableHead>
							<span className="inline-flex items-center gap-1">
								Budget type
								<InfoTooltip text="Users with group type will inherit the group budget allowance.

Users with individual type have a budget override." />
							</span>
						</TableHead>
						<TableHead className="w-auto" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{membersWithBudget.length === 0 ? (
						<TableRow>
							<TableCell colSpan={999}>
								<EmptyState message="No members found" />
							</TableCell>
						</TableRow>
					) : (
						membersWithBudget.map((mwb) => {
							const { member, budget, roles } = mwb;
							const isOverBudget =
								budget != null &&
								budget.spentUSD / budget.limitUSD >=
									BUDGET_WARNING_THRESHOLD;

							return (
								<TableRow key={member.id}>
									<TableCell>
										<AvatarData
											avatar={
												<div
													className={cn(
															"rounded-[8px]",
															isOverBudget &&
																"ring-2 ring-content-danger ring-offset-1 ring-offset-surface-primary",
													)}
												>
													<Avatar
														size="lg"
														fallback={member.username}
														src={member.avatar_url}
													/>
												</div>
											}
											title={member.username}
											subtitle={
												member.is_service_account
													? "Service Account"
													: member.email
											}
										/>
									</TableCell>

									<TableCell>
										<div className="flex items-center gap-1">
											{roles?.map((r) => (
												<Badge key={r.name} size="sm">
													{r.name}
												</Badge>
											))}
										</div>
									</TableCell>

									<TableCell
										className={cn(
											"capitalize",
													member.status === "suspended" &&
												"text-content-secondary",
												)}
											>
												<div>{member.status}</div>
												{(member.status === "active" ||
													member.status === "dormant") && (
											<LastSeen
												at={member.last_seen_at}
												className="text-xs"
													/>
												)}
									</TableCell>

									<TableCell>
										{budget ? (
											<span className="inline-flex items-center gap-1">
												<span
													className={cn(
														"tabular-nums",
														isOverBudget
															? "text-content-destructive"
															: "text-content-primary",
													)}
												>
													{formatUSD(
														budget.breakdown
															? budget.breakdown
																	.filter((b) => b.groupName === "DevOps")
																	.reduce((sum, b) => sum + b.amountUSD, 0)
															: budget.spentUSD,
													)}
												</span>
												<span className="text-content-secondary">
													{" / "}
													{budget.limitUSD.toLocaleString(
														"en-US",
													)}{" "}
													USD
												</span>
												{budget.budgetType === "individual" &&
													budget.breakdown && (
														<BudgetBreakdownTooltip
															memberName={member.username}
															groupName="DevOps"
															breakdown={budget.breakdown}
														>
															<InfoIcon className="size-icon-xs text-content-secondary cursor-pointer hover:text-content-primary" />
														</BudgetBreakdownTooltip>
													)}
											</span>
										) : (
											<span className="inline-flex items-center gap-1.5 text-content-secondary">
												&mdash;
													<Tooltip>
														<TooltipTrigger asChild>
															<InfoIcon className="size-icon-xs text-content-secondary cursor-default" />
														</TooltipTrigger>
														<TooltipContent align="end">
															This users budget is attributed to Pineapple group.
														</TooltipContent>
													</Tooltip>
											</span>
										)}
									</TableCell>

									<TableCell>
										{budget ? (
											<div className="inline-flex items-center gap-1.5">
												<Badge size="sm">
													{budget.budgetType === "group"
														? "Group"
														: "Individual"}
												</Badge>
											</div>
										) : null}
									</TableCell>

									<TableCell>
										<DropdownMenu>
											<DropdownMenuTrigger asChild>
												<Button
													size="icon-lg"
													variant="subtle"
													aria-label="Open menu"
												>
													<EllipsisVerticalIcon aria-hidden="true" />
													<span className="sr-only">
														Open menu
													</span>
												</Button>
											</DropdownMenuTrigger>
											<DropdownMenuContent align="end">
												<DropdownMenuItem
													onClick={() => {
														setSelectedMember(mwb);
														setSpendLimitOpen(true);
													}}
												>
													Update budget...
												</DropdownMenuItem>
												<DropdownMenuItem>
													View session logs
												</DropdownMenuItem>
												<DropdownMenuItem>
													View group details
												</DropdownMenuItem>
												<DropdownMenuItem className="text-content-destructive focus:text-content-destructive">
													Remove AI access
												</DropdownMenuItem>
											</DropdownMenuContent>
										</DropdownMenu>
									</TableCell>
								</TableRow>
							);
						})
					)}
				</TableBody>
			</Table>

			{selectedMember?.budget && (
				<AISpendLimitDialog
					open={spendLimitOpen}
					onOpenChange={setSpendLimitOpen}
					memberName={selectedMember.member.username}
					memberEmail={selectedMember.member.email}
					memberAvatarUrl={selectedMember.member.avatar_url}
					budget={
						selectedMember.budget.customMonthlyBudget ??
						selectedMember.budget.limitUSD
					}
					hasCustomBudget={selectedMember.budget.customMonthlyBudget != null}
					assignedGroup={
						selectedMember.budget.attributedGroup ?? "Personal"
					}
					groups={MOCK_GROUPS}
				/>
			)}
		</>
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
