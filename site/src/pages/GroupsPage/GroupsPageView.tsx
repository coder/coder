import {
	ChevronRightIcon,
	InfoIcon,
	PlusIcon,
	SearchIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { Group } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ChooseOne, Cond } from "#/components/Conditionals/ChooseOne";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Input } from "#/components/Input/Input";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { cn } from "#/utils/cn";
import { docs } from "#/utils/docs";

// Budget information for a group's AI spending.
export type GroupBudgetInfo = {
	spentUSD: number;
	// When null, the budget is unlimited.
	limitUSD: number | null;
	aiSeats: number;
};

type GroupsPageViewProps = {
	groups: Group[] | undefined;
	canCreateGroup: boolean;
	groupsEnabled: boolean;
	// Maps group ID to budget info. When provided, the AI budget
	// and AI seats columns are displayed.
	budgets?: Record<string, GroupBudgetInfo>;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
	groups,
	canCreateGroup,
	groupsEnabled,
	budgets,
}) => {
	const isLoading = Boolean(groups === undefined);
	const isEmpty = Boolean(groups && groups.length === 0);
	const showBudgets = budgets !== undefined;

	return (
		<ChooseOne>
			<Cond condition={!groupsEnabled}>
				<PaywallPremium
					message="Groups"
					description="Organize users into groups with restricted access to templates. You need a Premium license to use this feature."
					documentationLink={docs("/admin/users/groups-roles")}
				/>
			</Cond>
			<Cond>
				<div className="flex flex-col gap-4">
					<div className="relative max-w-sm">
						<SearchIcon className="absolute left-3 top-1/2 size-icon-sm -translate-y-1/2 text-content-secondary" />
						<Input
							placeholder="Search groups..."
							className="pl-10"
						/>
					</div>

					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="w-2/6">Name</TableHead>
								<TableHead className="w-2/6">Users</TableHead>
								{showBudgets && (
									<>
										<TableHead>
											<span className="inline-flex items-center gap-1">
												AI budget
												<InfoTooltip text="Total monthly budget attributed to this group" />
											</span>
										</TableHead>
										<TableHead>
											<span className="inline-flex items-center gap-1">
												AI seats
												<InfoTooltip text="Active seats using AI Governance in the last 48 hours" />
											</span>
										</TableHead>
									</>
								)}
								<TableHead className="w-auto" />
							</TableRow>
						</TableHeader>
						<TableBody>
							<ChooseOne>
								<Cond condition={isLoading}>
									<TableLoader showBudgets={showBudgets} />
								</Cond>

								<Cond condition={isEmpty}>
									<TableRow>
										<TableCell colSpan={999}>
											<EmptyState
												message="No groups yet"
												description={
													canCreateGroup
														? "Create your first group"
														: "You don't have permission to create a group"
												}
												cta={
													canCreateGroup && (
														<Button asChild>
															<RouterLink to="create">
																<PlusIcon className="size-icon-sm" />
																Create group
															</RouterLink>
														</Button>
													)
												}
											/>
										</TableCell>
									</TableRow>
								</Cond>

								<Cond>
									{groups?.map((group) => (
										<GroupRow
											key={group.id}
											group={group}
											budget={budgets?.[group.id]}
										/>
									))}
								</Cond>
							</ChooseOne>
						</TableBody>
					</Table>

					{groups && groups.length > 0 && (
						<p className="text-right text-sm text-content-secondary">
							Showing <span className="font-semibold text-content-primary">{groups.length}</span> of{" "}
							<span className="font-semibold text-content-primary">{groups.length}</span> groups
						</p>
					)}
				</div>
			</Cond>
		</ChooseOne>
	);
};

const InfoTooltip: FC<{ text: string }> = ({ text }) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<InfoIcon className="size-icon-xs text-content-secondary cursor-default" />
		</TooltipTrigger>
		<TooltipContent align="end">{text}</TooltipContent>
	</Tooltip>
);

interface GroupRowProps {
	group: Group;
	budget?: GroupBudgetInfo;
}

// The threshold at which budget text turns into a warning color.
const BUDGET_WARNING_THRESHOLD = 0.90;

const formatUSD = (amount: number): string => {
	return `$${amount.toLocaleString("en-US")}`;
};

const GroupRow: FC<GroupRowProps> = ({ group, budget }) => {
	const navigate = useNavigate();
	const rowProps = useClickableTableRow({
		onClick: () => navigate(group.name),
	});
	const memberAvatars = group.members.slice(0, 5);
	const remainingAvatars = group.members.length - memberAvatars.length;

	const isOverBudget =
		budget?.limitUSD != null &&
		budget.spentUSD / budget.limitUSD >= BUDGET_WARNING_THRESHOLD;

	return (
		<TableRow data-testid={`group-${group.id}`} {...rowProps}>
			<TableCell>
				<AvatarData
					avatar={
						<Avatar
							size="lg"
							variant="icon"
							fallback={group.display_name || group.name}
							src={group.avatar_url}
						/>
					}
					title={group.display_name || group.name}
					subtitle={`${group.members.length} members`}
				/>
			</TableCell>

			<TableCell>
				{group.members.length > 0 ? (
					<div className="flex items-center gap-2">
						{memberAvatars.map((member) => (
							<Avatar
								key={member.username}
								fallback={member.username}
								src={member.avatar_url}
							/>
						))}
						{remainingAvatars > 0 && (
							<Badge className="h-[--avatar-default]">
								+{remainingAvatars}
							</Badge>
						)}
					</div>
				) : (
					"-"
				)}
			</TableCell>

			{budget && (
				<>
					<TableCell>
						<span
							className={cn(
								"tabular-nums",
								isOverBudget
									? "text-content-destructive"
									: "text-content-primary",
							)}
						>
							{formatUSD(budget.spentUSD)}
						</span>
						<span className="text-content-secondary">
							{" / "}
							{budget.limitUSD != null
								? `${budget.limitUSD.toLocaleString("en-US")} USD`
								: "unlimited USD"}
						</span>
					</TableCell>
					<TableCell>
						<span className="tabular-nums">{budget.aiSeats}</span>
					</TableCell>
				</>
			)}

			<TableCell>
				<div className="flex justify-end">
					<ChevronRightIcon className="size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};

const TableLoader: FC<{ showBudgets?: boolean }> = ({ showBudgets }) => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<div className="flex items-center gap-2">
						<AvatarDataSkeleton />
					</div>
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				{showBudgets && (
					<>
						<TableCell>
							<Skeleton variant="text" width="60%" />
						</TableCell>
						<TableCell>
							<Skeleton variant="text" width="25%" />
						</TableCell>
					</>
				)}
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
