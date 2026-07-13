import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import type { GroupMemberAICostControl } from "#/api/api";
import { groupById } from "#/api/queries/groups";
import type { Group } from "#/api/typesGenerated";
import { AIBudgetAmount } from "#/components/AIBudgetAmount/AIBudgetAmount";
import { Badge } from "#/components/Badge/Badge";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableCell } from "#/components/Table/Table";
import { formatBudgetUSD } from "#/utils/currency";
import { InfoIconTooltip } from "./InfoIconTooltip";

const EM_DASH = "\u2014";

/**
 * The AI budget and Budget group cells for a group member. Spend only counts
 * against the viewed group; another group's budget shows as unattributed.
 */
export const GroupMemberBudgetCells: FC<{
	group: Group;
	userID: string;
	costControl: GroupMemberAICostControl | undefined;
}> = ({ group, userID, costControl }) => {
	const effective = effectiveBudgetGroup(costControl, group);
	const fromOtherGroup = effective.kind === "other";

	const { data: effectiveGroup, isLoading: isResolvingGroupName } = useQuery({
		...groupById(fromOtherGroup ? effective.groupId : "", {
			exclude_members: true,
		}),
		enabled: fromOtherGroup,
	});
	const effectiveGroupName =
		effectiveGroup?.display_name || effectiveGroup?.name;
	const groupName = group.display_name || group.name;
	// A user override shows as "(individual)" on the governing group's badge.
	const badgeName = (name: string) =>
		costControl?.limit_source === "user_override"
			? `${name} (individual)`
			: name;

	let budgetGroup: ReactNode;
	switch (effective.kind) {
		case "none":
			budgetGroup = EM_DASH;
			break;
		case "everyone":
			budgetGroup = <Badge size="sm">Everyone (not allocated)</Badge>;
			break;
		case "this":
			budgetGroup = <Badge size="sm">{badgeName(groupName)}</Badge>;
			break;
		case "other": {
			// "Another org" when the governing group can't be resolved.
			const label = effectiveGroupName
				? badgeName(effectiveGroupName)
				: "Another org";
			// Wait for the name to resolve rather than flashing the fallback.
			budgetGroup = isResolvingGroupName ? (
				<Spinner loading size="sm" />
			) : (
				<Badge size="sm">{label}</Badge>
			);
			break;
		}
	}

	let budget: ReactNode = EM_DASH;
	if (costControl && fromOtherGroup) {
		if (isResolvingGroupName) {
			budget = <Spinner loading size="sm" />;
		} else if (!effectiveGroupName) {
			// The spend hides entirely when the governing group can't be resolved.
			budget = (
				<LabelWithInfo
					label={EM_DASH}
					message="This user's AI budget is managed by another org and isn't visible here."
				/>
			);
		} else {
			budget = (
				<div className="flex flex-col gap-0.5">
					<span className="flex items-center gap-1">
						<span>
							<span className="text-content-secondary">
								{formatBudgetUSD(costControl.current_spend_micros)}
							</span>{" "}
							<span className="text-content-disabled">USD</span>
						</span>
						<InfoIconTooltip
							message={
								<>
									None of this user's spend counts against the{" "}
									<span className="font-medium text-content-primary">
										{groupName}
									</span>{" "}
									group. It is managed by the{" "}
									<span className="font-medium text-content-primary">
										{effectiveGroupName}
									</span>{" "}
									group.
								</>
							}
						/>
					</span>
					<span className="text-xs text-content-secondary">
						Not attributed to this group
					</span>
				</div>
			);
		}
	} else if (costControl) {
		const limit = costControl.spend_limit_micros;
		if (limit === null) {
			// Also covers a missing governing group: no budget applies.
			budget = (
				<LabelWithInfo
					label="Unlimited"
					message="None of this user's groups have an AI budget configured, so their AI usage isn't restricted."
				/>
			);
		} else if (limit === 0) {
			// A $0 budget disables spending, distinct from no budget configured.
			budget = (
				<LabelWithInfo
					label="None"
					message="This user's group(s) have an AI budget of $0, so they have no AI spending allowance."
				/>
			);
		} else {
			const limitLabel =
				costControl.limit_source === "user_override" ? "Custom" : "Group";
			budget = (
				<div className="flex flex-col gap-0.5">
					<span>
						<AIBudgetAmount
							spend={costControl.current_spend_micros}
							limit={limit}
						/>{" "}
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
	| { kind: "other"; groupId: string };

/**
 * Resolves which group governs a member's AI budget. "none" means no budget
 * applies; "everyone" is the org-wide fallback when no named group sets a
 * budget.
 *
 * TODO(AIGOV-509): null will instead mean a group in another org.
 */
export function effectiveBudgetGroup(
	costControl: GroupMemberAICostControl | undefined,
	group: Pick<Group, "id" | "organization_id">,
): EffectiveBudgetGroup {
	const groupId = costControl?.effective_group_id ?? null;
	if (groupId === null) {
		return { kind: "none" };
	}
	// Everyone shares the org's id; checked first so it wins when the viewed
	// group is Everyone itself.
	if (groupId === group.organization_id) {
		return { kind: "everyone" };
	}
	if (groupId === group.id) {
		return { kind: "this" };
	}
	return { kind: "other", groupId };
}

const LabelWithInfo: FC<{ label: ReactNode; message: ReactNode }> = ({
	label,
	message,
}) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<InfoIconTooltip message={message} />
	</span>
);
