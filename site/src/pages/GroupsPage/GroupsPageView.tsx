import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { OrganizationGroupAISpend } from "#/api/api";
import type { Group } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
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
import { microsToDollars, usdBudgetFormatter } from "#/utils/currency";
import { docs } from "#/utils/docs";
import { getSeverity, severityTextClassName } from "#/utils/usage";

type GroupsPageViewProps = {
	groups: Group[] | undefined;
	canCreateGroup: boolean;
	groupsEnabled: boolean;
	// Present when the AI budget column should be shown.
	aiBudget?: {
		spend: readonly OrganizationGroupAISpend[] | undefined;
		isLoading: boolean;
	};
};

// Per-group spend resolved for rendering; present only when the column shows.
type AIBudgetColumn = {
	spendByGroupID: ReadonlyMap<string, OrganizationGroupAISpend>;
	isLoading: boolean;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
	groups,
	canCreateGroup,
	groupsEnabled,
	aiBudget,
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

	const aiBudgetColumn: AIBudgetColumn | undefined = aiBudget && {
		spendByGroupID: new Map(
			aiBudget.spend?.map((spend) => [spend.group_id, spend]),
		),
		isLoading: aiBudget.isLoading,
	};

	return (
		<Table aria-label="Groups">
			<TableHeader>
				<TableRow>
					<TableHead className="w-2/5">Name</TableHead>
					<TableHead className={aiBudgetColumn ? "w-1/5" : "w-3/5"}>
						Users
					</TableHead>
					{aiBudgetColumn && (
						<TableHead className="w-2/5">
							<div className="flex items-center gap-1">
								AI budget
								<InfoTooltip message="Current AI spend compared to the group's AI budget for the active period." />
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
					aiBudgetColumn={aiBudgetColumn}
				/>
			</TableBody>
		</Table>
	);
};

interface GroupsTableBodyProps {
	groups: Group[] | undefined;
	canCreateGroup: boolean;
	aiBudgetColumn: AIBudgetColumn | undefined;
}

const GroupsTableBody: FC<GroupsTableBodyProps> = ({
	groups,
	canCreateGroup,
	aiBudgetColumn,
}) => {
	if (groups === undefined) {
		return <TableLoader showAIBudget={aiBudgetColumn !== undefined} />;
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
				<GroupRow
					key={group.id}
					group={group}
					aiBudgetColumn={aiBudgetColumn}
				/>
			))}
		</>
	);
};

interface GroupRowProps {
	group: Group;
	aiBudgetColumn: AIBudgetColumn | undefined;
}

const GroupRow: FC<GroupRowProps> = ({ group, aiBudgetColumn }) => {
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

			{aiBudgetColumn && (
				<TableCell>
					<GroupAIBudgetCell
						aiSpend={aiBudgetColumn.spendByGroupID.get(group.id)}
						isLoading={aiBudgetColumn.isLoading}
					/>
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

const GroupAIBudgetCell: FC<{
	aiSpend: OrganizationGroupAISpend | undefined;
	isLoading: boolean;
}> = ({ aiSpend, isLoading }) => {
	if (isLoading) {
		return <Skeleton variant="text" width="50%" />;
	}

	if (aiSpend === undefined) {
		return "-";
	}

	const { current_spend_micros, spend_limit_micros } = aiSpend;

	return (
		<span className="whitespace-nowrap">
			<span
				className={severityTextClassName(
					spend_limit_micros === null
						? "normal"
						: getSeverity(current_spend_micros, spend_limit_micros),
				)}
			>
				{formatBudgetUSD(current_spend_micros)}
			</span>{" "}
			/{" "}
			{spend_limit_micros === null
				? "unlimited"
				: formatBudgetUSD(spend_limit_micros)}{" "}
			USD
		</span>
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

function formatBudgetUSD(micros: number): string {
	return usdBudgetFormatter.format(microsToDollars(micros));
}
