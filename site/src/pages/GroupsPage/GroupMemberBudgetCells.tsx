import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
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
	// A user override shows as "(individual)" on the governing group's badge.
	const badgeName = (name: string) =>
		spend?.group_budget?.limit_source === "user_override"
			? `${name} (individual)`
			: name;

	let budgetGroup: ReactNode;
	switch (effective.kind) {
		case "none":
			budgetGroup = EM_DASH;
			break;
		case "everyone":
			// A populated budget means the Everyone group's own budget applies,
			// so it isn't the unallocated fallback.
			budgetGroup = (
				<Badge size="sm">
					{spend?.group_budget
						? badgeName("Everyone")
						: "Everyone (not allocated)"}
				</Badge>
			);
			break;
		case "this":
			budgetGroup = <Badge size="sm">{badgeName(groupName)}</Badge>;
			break;
		case "other":
			// Wait for the name to resolve rather than flashing the fallback.
			if (isResolvingGroupName) {
				budgetGroup = <Spinner loading size="sm" />;
			} else {
				budgetGroup = effectiveGroupName ? (
					<Badge size="sm">{badgeName(effectiveGroupName)}</Badge>
				) : (
					EM_DASH
				);
			}
			break;
		case "otherOrganization":
			budgetGroup = (
				<LabelWithInfo label={EM_DASH} message={OTHER_ORG_MESSAGE} />
			);
			break;
	}

	let budget: ReactNode = EM_DASH;
	if (spend && effective.kind === "otherOrganization") {
		budget = <LabelWithInfo label={EM_DASH} message={OTHER_ORG_MESSAGE} />;
	} else if (spend && effective.kind === "other") {
		if (isResolvingGroupName) {
			budget = <Spinner loading size="sm" />;
		} else if (effectiveGroupName) {
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
	| { kind: "other" }
	| { kind: "otherOrganization" };

/**
 * Resolves which group governs a member's AI budget:
 *
 * - "none": spend data is not loaded.
 * - "everyone": the Everyone group governs the budget.
 * - "this": the viewed group governs the budget.
 * - "other": another group in this organization governs the budget.
 * - "otherOrganization": a group in another organization governs the budget.
 */
export function effectiveBudgetGroup(
	spend: GroupMemberAISpend | undefined,
	group: Pick<Group, "id" | "organization_id">,
): EffectiveBudgetGroup {
	const groupId = spend?.effective_group_id ?? null;
	if (groupId === null) {
		return spend === undefined
			? { kind: "none" }
			: { kind: "otherOrganization" };
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

const LabelWithInfo: FC<{ label: ReactNode; message: ReactNode }> = ({
	label,
	message,
}) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<StatusIconTooltip message={message} />
	</span>
);
