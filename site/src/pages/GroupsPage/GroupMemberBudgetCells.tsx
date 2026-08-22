import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import { groupById } from "#/api/queries/groups";
import type { Group, GroupMemberAISpend } from "#/api/typesGenerated";
import { AIBudgetAmount } from "#/components/AIBudgetAmount/AIBudgetAmount";
import { AIBudgetUsage } from "#/components/AIBudgetUsage/AIBudgetUsage";
import { Badge } from "#/components/Badge/Badge";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableCell } from "#/components/Table/Table";
import { formatBudgetUSD } from "#/utils/currency";
import { StatusIconTooltip } from "./StatusIconTooltip";

const EM_DASH = "\u2014";

/** Shown on both cells when the governing group is in another org. */
const OTHER_ORG_MESSAGE =
	"This user's AI budget is managed by a group in another organization and isn't visible here.";

/**
 * The AI spend and Budget group cells for a group member. Spend is scoped to
 * the viewed group; the limit comes from the member's effective group.
 */
export const GroupMemberBudgetCells: FC<{
	group: Group;
	userID: string;
	spend: GroupMemberAISpend | undefined;
}> = ({ group, userID, spend }) => {
	const effective = effectiveBudgetGroup(spend, group);
	const fromOtherGroup = effective.kind === "other";

	// A null effective_group_id is a group in another org that can't be
	// fetched, so only resolve the name when an ID exists.
	const { data: effectiveGroup, isLoading: isResolvingGroupName } = useQuery({
		...groupById(spend?.effective_group_id ?? "", {
			exclude_members: true,
		}),
		enabled: fromOtherGroup && Boolean(spend?.effective_group_id),
	});
	const effectiveGroupName =
		effectiveGroup?.display_name || effectiveGroup?.name;
	const groupName = group.display_name || group.name;
	// The members route resolves the :groupName segment by group name (not id),
	// so links must use the group's name. The Everyone group is always named
	// "Everyone".
	const hrefForGroup = (routeName: string) =>
		`/organizations/${group.organization_name}/groups/${encodeURIComponent(routeName)}`;
	// A user override shows as "(individual)" next to the governing group's name.
	const individualSuffix =
		spend?.group_budget?.limit_source === "user_override"
			? " (individual)"
			: "";

	let budgetGroup: ReactNode;
	switch (effective.kind) {
		case "none":
			budgetGroup = EM_DASH;
			break;
		case "everyone":
			// A populated budget means the Everyone group's own budget applies,
			// so it isn't the unallocated fallback.
			budgetGroup = (
				<BudgetGroupBadge
					name="Everyone"
					href={hrefForGroup("Everyone")}
					suffix={spend?.group_budget ? individualSuffix : " (not allocated)"}
				/>
			);
			break;
		case "this":
			budgetGroup = (
				<BudgetGroupBadge
					name={groupName}
					href={hrefForGroup(group.name)}
					suffix={individualSuffix}
				/>
			);
			break;
		case "other": {
			// Wait for the name to resolve rather than flashing the fallback.
			if (isResolvingGroupName) {
				budgetGroup = <Spinner loading size="sm" />;
			} else if (effectiveGroup) {
				budgetGroup = (
					<BudgetGroupBadge
						name={effectiveGroupName ?? effectiveGroup.name}
						href={hrefForGroup(effectiveGroup.name)}
						suffix={individualSuffix}
					/>
				);
			} else {
				// The group can't be resolved (another org), so it can't be named.
				budgetGroup = (
					<LabelWithInfo label={EM_DASH} message={OTHER_ORG_MESSAGE} />
				);
			}
			break;
		}
	}

	let budget: ReactNode = EM_DASH;
	if (spend && fromOtherGroup) {
		if (isResolvingGroupName) {
			budget = <Spinner loading size="sm" />;
		} else if (!effectiveGroupName) {
			// The spend hides entirely when the governing group can't be resolved.
			budget = <LabelWithInfo label={EM_DASH} message={OTHER_ORG_MESSAGE} />;
		} else {
			budget = (
				<div className="flex flex-col gap-0.5">
					<span className="flex items-center gap-1">
						<span>
							<span className="text-content-secondary">
								{formatBudgetUSD(spend.group_spend_micros)}
							</span>{" "}
							<span className="text-content-disabled">USD</span>
						</span>
						<StatusIconTooltip
							message={
								<>
									The amount shown is this user's spend in the{" "}
									<span className="font-medium text-content-primary">
										{groupName}
									</span>{" "}
									group. Their AI budget is currently managed by the{" "}
									<span className="font-medium text-content-primary">
										{effectiveGroupName}
									</span>{" "}
									group.
								</>
							}
						/>
					</span>
					<span className="text-xs text-content-secondary">
						Budget managed by another group
					</span>
				</div>
			);
		}
	} else if (spend) {
		const limit = spend.group_budget?.spend_limit_micros ?? null;
		if (limit === null) {
			// The effective group has no budget, so no limit applies.
			budget = (
				<LabelWithInfo
					label={
						<AIBudgetUsage
							currentSpend={spend.group_spend_micros}
							spendLimit={null}
						/>
					}
					message="None of this user's groups have an AI budget configured, so their AI usage isn't restricted."
				/>
			);
		} else {
			const limitLabel =
				spend.group_budget?.limit_source === "user_override"
					? "Custom"
					: "Group";
			budget = (
				<div className="flex flex-col gap-0.5">
					<span>
						<AIBudgetAmount spend={spend.group_spend_micros} limit={limit} />{" "}
						<span className="text-content-disabled">USD</span>
					</span>
					<span className="text-xs text-content-secondary">
						{`${limitLabel} limit ${formatBudgetUSD(limit)}`}
					</span>
				</div>
			);
		}
	}

	return (
		<>
			<TableCell
				data-testid={`member-ai-budget-${userID}`}
				className="whitespace-nowrap tabular-nums"
			>
				{budget}
			</TableCell>
			<TableCell>{budgetGroup}</TableCell>
		</>
	);
};

/** Which group governs a member's AI budget, relative to the given group. */
type EffectiveBudgetGroup =
	| { kind: "none" }
	| { kind: "everyone" }
	| { kind: "this" }
	| { kind: "other" };

/**
 * Resolves which group governs a member's AI budget. "none" means no budget
 * data loaded; "everyone" is the org-wide fallback when no named group sets a
 * budget. A null effective group means the budget resolves to a group in
 * another organization, so it can't be shown here.
 */
export function effectiveBudgetGroup(
	spend: GroupMemberAISpend | undefined,
	group: Pick<Group, "id" | "organization_id">,
): EffectiveBudgetGroup {
	const groupId = spend?.effective_group_id ?? null;
	if (groupId === null) {
		return spend === undefined ? { kind: "none" } : { kind: "other" };
	}
	// Everyone shares the org's id; checked first so it wins when the viewed
	// group is Everyone itself.
	if (groupId === group.organization_id) {
		return { kind: "everyone" };
	}
	if (groupId === group.id) {
		return { kind: "this" };
	}
	return { kind: "other" };
}

/**
 * A Budget group badge whose group name links to that group's page. Only the
 * name is a link; any suffix such as "(individual)" stays plain text so it is
 * excluded from the link target.
 */
const BudgetGroupBadge: FC<{
	name: string;
	href: string | undefined;
	suffix?: string;
}> = ({ name, href, suffix }) => (
	<Badge size="sm">
		{/* One inline span keeps the badge's flex gap off the name/suffix pair. */}
		<span>
			{href ? (
				<RouterLink
					to={href}
					className="text-inherit no-underline hover:underline"
				>
					{name}
				</RouterLink>
			) : (
				name
			)}
			{suffix}
		</span>
	</Badge>
);

const LabelWithInfo: FC<{ label: ReactNode; message: ReactNode }> = ({
	label,
	message,
}) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<StatusIconTooltip message={message} />
	</span>
);
