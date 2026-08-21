import { ChevronRightIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useNavigate } from "react-router";
import {
	GROUP_MEMBER_AVATAR_LIMIT,
	groupMemberAvatars,
} from "#/api/queries/groups";
import type {
	OrganizationGroupsAISpend,
	PaginatedGroup,
} from "#/api/typesGenerated";
import { AIBudgetUsage } from "#/components/AIBudgetUsage/AIBudgetUsage";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import type { useFilter } from "#/components/Filter/Filter";
import { GroupsFilter } from "#/components/Filter/GroupsFilter";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
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
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import type { PaginationResultInfo } from "#/hooks/usePaginatedQuery";
import type { Permissions } from "#/modules/permissions";
import { SpendEstimateDocsLink } from "./AICostControl";
import { StatusIconTooltip } from "./StatusIconTooltip";

const EM_DASH = "\u2014";

// Stable keys for the avatar loading skeletons (indexes would trip lint).
const AVATAR_SKELETON_KEYS = ["a", "b", "c", "d", "e"];

export type GroupWithSpend = PaginatedGroup & {
	readonly spend: OrganizationGroupsAISpend["groups"][number] | undefined;
};

/** Attach each group's spend, when present, so rows get a single object. */
export const joinGroupsSpend = (
	groups: readonly PaginatedGroup[] | undefined,
	groupsSpend: OrganizationGroupsAISpend | undefined,
): GroupWithSpend[] | undefined => {
	if (groups === undefined) {
		return undefined;
	}
	const spendByGroupId = new Map(
		groupsSpend?.groups.map((spend) => [spend.group_id, spend]) ?? [],
	);
	return groups.map((group) => ({
		...group,
		spend: spendByGroupId.get(group.id),
	}));
};

type GroupsPageViewProps = {
	groups: GroupWithSpend[] | undefined;
	/** True when the spend query failed; cells then show an em dash. */
	spendError: boolean;
	canCreateGroup: boolean;
	groupsEnabled: boolean;
	showAIBudget: boolean;
	filterProps: { filter: ReturnType<typeof useFilter> };
	groupsQuery: PaginationResultInfo & {
		isPlaceholderData: boolean;
	};
	permissions: Permissions;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
	groups,
	spendError,
	canCreateGroup,
	groupsEnabled,
	showAIBudget,
	filterProps,
	groupsQuery,
	permissions,
}) => {
	if (!groupsEnabled) {
		return (
			<PaywallPremium
				message="Groups"
				description="Run isolated business units on one deployment, each with its own users, templates, provisioners, and infrastructure."
				features={[
					"Isolate provisioners & infrastructure",
					"Sync org membership from your IdP",
					"Manage orgs at scale via Terraform",
				]}
				canViewPremium={permissions.viewAllLicenses}
			/>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<div className="flex flex-row justify-between">
				<GroupsFilter {...filterProps} />
				{canCreateGroup && (
					<Button asChild>
						<RouterLink to="create">
							<PlusIcon className="size-icon-sm" />
							Create group
						</RouterLink>
					</Button>
				)}
			</div>

			<PaginationContainer query={groupsQuery} paginationUnitLabel="groups">
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
										AI spend
										{spendError ? (
											<StatusIconTooltip
												kind="warning"
												message="AI spend couldn't be loaded, so budgets aren't shown."
											/>
										) : (
											<StatusIconTooltip
												message={
													<>
														Approximate AI spend compared to the group's AI
														budget for the active period.{" "}
														<SpendEstimateDocsLink />
													</>
												}
											/>
										)}
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
							filterUsed={filterProps.filter.used}
						/>
					</TableBody>
				</Table>
			</PaginationContainer>
		</div>
	);
};

interface GroupsTableBodyProps {
	groups: GroupWithSpend[] | undefined;
	canCreateGroup: boolean;
	showAIBudget: boolean;
	filterUsed: boolean;
}

const GroupsTableBody: FC<GroupsTableBodyProps> = ({
	groups,
	canCreateGroup,
	showAIBudget,
	filterUsed,
}) => {
	if (groups === undefined) {
		return <TableLoader showAIBudget={showAIBudget} />;
	}
	if (groups.length === 0) {
		// When a search returned no matches, don't nudge the user to create a
		// first group; the org may already have groups that simply don't match.
		if (filterUsed) {
			return (
				<TableRow>
					<TableCell colSpan={999}>
						<EmptyState
							message="No groups match your search"
							description="Try a different search term."
						/>
					</TableCell>
				</TableRow>
			);
		}
		return (
			<TableEmpty
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
	group: GroupWithSpend;
	showAIBudget: boolean;
}

const GroupRow: FC<GroupRowProps> = ({ group, showAIBudget }) => {
	const navigate = useNavigate();
	const rowProps = useClickableTableRow({
		onClick: () => navigate(group.name),
	});

	// The list endpoint returns only total_member_count, so fetch a small
	// avatar preview per visible row instead of a full roster.
	const membersQuery = useQuery({
		...groupMemberAvatars(
			group.organization_name,
			group.name,
			GROUP_MEMBER_AVATAR_LIMIT,
		),
		enabled: group.total_member_count > 0,
	});
	const memberAvatars = membersQuery.data?.users ?? [];
	const remainingAvatars = group.total_member_count - memberAvatars.length;
	const skeletonCount = Math.min(
		group.total_member_count,
		GROUP_MEMBER_AVATAR_LIMIT,
	);

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
					subtitle={`${group.total_member_count} members`}
				/>
			</TableCell>

			<TableCell>
				{group.total_member_count === 0 || membersQuery.isError ? (
					EM_DASH
				) : membersQuery.isLoading ? (
					<div className="flex items-center gap-2">
						{AVATAR_SKELETON_KEYS.slice(0, skeletonCount).map((key) => (
							<Skeleton
								key={key}
								variant="circular"
								className="size-[--avatar-default]"
							/>
						))}
					</div>
				) : (
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
				)}
			</TableCell>

			{showAIBudget && (
				<TableCell>
					{group.spend ? (
						<AIBudgetUsage
							currentSpend={group.spend.current_spend_micros}
							spendLimit={group.spend.total_spend_limit_micros}
						/>
					) : (
						EM_DASH
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
