import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { GroupWithAICostControl } from "#/api/api";
import { AIBudgetUsage } from "#/components/AIBudgetUsage/AIBudgetUsage";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
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
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { docs } from "#/utils/docs";
import { InfoIconTooltip } from "./InfoIconTooltip";

type GroupsPageViewProps = {
	groups: GroupWithAICostControl[] | undefined;
	canCreateGroup: boolean;
	groupsEnabled: boolean;
	showAIBudget: boolean;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
	groups,
	canCreateGroup,
	groupsEnabled,
	showAIBudget,
}) => {
	if (!groupsEnabled) {
		return (
			<PaywallPremium
				message="Groups"
				description="Organize users into groups with restricted access to templates. You need a Premium license to use this feature."
				documentationLink={docs("/admin/users/groups-roles")}
			/>
		);
	}

	return (
		<Table aria-label="Groups">
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">Name</TableHead>
					<TableHead className={showAIBudget ? "w-1/5" : "w-3/5"}>
						Users
					</TableHead>
					{showAIBudget && (
						<TableHead className="w-2/5">
							<div className="flex items-center gap-1">
								AI budget
								<InfoIconTooltip message="Current AI spend compared to the group's AI budget for the active period." />
							</div>
						</TableHead>
					)}
					<TableHead className="w-auto" />
				</TableRow>
			</TableHeader>
			<TableBody>
				<GroupsTableBody
					groups={groups}
					canCreateGroup={canCreateGroup}
					showAIBudget={showAIBudget}
				/>
			</TableBody>
		</Table>
	);
};

interface GroupsTableBodyProps {
	groups: GroupWithAICostControl[] | undefined;
	canCreateGroup: boolean;
	showAIBudget: boolean;
}

const GroupsTableBody: FC<GroupsTableBodyProps> = ({
	groups,
	canCreateGroup,
	showAIBudget,
}) => {
	if (groups === undefined) {
		return <TableLoader showAIBudget={showAIBudget} />;
	}
	if (groups.length === 0) {
		return (
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
		);
	}
	return (
		<>
			{groups.map((group) => (
				<GroupRow key={group.id} group={group} showAIBudget={showAIBudget} />
			))}
		</>
	);
};

interface GroupRowProps {
	group: GroupWithAICostControl;
	showAIBudget: boolean;
}

const GroupRow: FC<GroupRowProps> = ({ group, showAIBudget }) => {
	const navigate = useNavigate();
	const rowProps = useClickableTableRow({
		onClick: () => navigate(group.name),
	});
	const memberAvatars = group.members.slice(0, 5);
	const remainingAvatars = group.members.length - memberAvatars.length;

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

			{showAIBudget && (
				<TableCell>
					{group.ai_cost_control ? (
						<AIBudgetUsage
							currentSpend={group.ai_cost_control.current_spend_micros}
							spendLimit={group.ai_cost_control.spend_limit_micros}
						/>
					) : (
						"-"
					)}
				</TableCell>
			)}

			<TableCell>
				<div className="flex">
					<ChevronRightIcon className="size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};

const TableLoader: FC<{ showAIBudget: boolean }> = ({ showAIBudget }) => {
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
				{showAIBudget && (
					<TableCell>
						<Skeleton variant="text" width="50%" />
					</TableCell>
				)}
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
