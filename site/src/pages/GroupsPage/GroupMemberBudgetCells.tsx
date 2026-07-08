import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import type { GroupMemberAICostControl } from "#/api/api";
import { groupById } from "#/api/queries/groups";
import type { Group } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableCell } from "#/components/Table/Table";
import { getSeverity, severityAmountClassName } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";
import { InfoIconTooltip } from "./InfoIconTooltip";

// Escaped so the emdash lint doesn't flag a literal.
export const emDash = "\u2014";

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
	const notAttributed = effective.kind === "none" || effective.kind === "other";

	const { data: effectiveGroup, isLoading: isResolvingGroupName } = useQuery({
		...groupById(effective.kind === "other" ? effective.groupId : "", {
			exclude_members: true,
		}),
		enabled: effective.kind === "other",
	});
	const effectiveGroupName =
		effectiveGroup?.display_name || effectiveGroup?.name;
	const groupName = group.display_name || group.name;

	let budgetGroup: ReactNode;
	switch (effective.kind) {
		case "none":
			budgetGroup = emDash;
			break;
		case "everyone":
			budgetGroup = <Badge size="sm">Everyone (not allocated)</Badge>;
			break;
		case "this":
		case "other": {
			const name = effective.kind === "this" ? groupName : effectiveGroupName;
			// "Another org" when the governing group can't be resolved.
			let label = "Another org";
			if (name) {
				label =
					costControl?.limit_source === "user_override"
						? `${name} (individual)`
						: name;
			}
			budgetGroup = <Badge size="sm">{label}</Badge>;
			break;
		}
	}

	return (
		<>
			<TableCell
				data-testid={`member-ai-budget-${userID}`}
				className="whitespace-nowrap tabular-nums"
			>
				{costControl ? (
					<BudgetAmount
						costControl={costControl}
						groupName={groupName}
						notAttributed={notAttributed}
						effectiveGroupName={effectiveGroupName}
						isResolvingGroupName={isResolvingGroupName}
					/>
				) : (
					emDash
				)}
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
 * Resolves which group governs a member's AI budget. "none" covers missing
 * cost control data as well as no governing group; "everyone" is the org-wide
 * fallback when no named group sets a budget.
 */
export function effectiveBudgetGroup(
	costControl: GroupMemberAICostControl | undefined,
	group: Pick<Group, "id" | "organization_id">,
): EffectiveBudgetGroup {
	const groupId = costControl?.effective_group_id ?? null;
	if (groupId === null) {
		return { kind: "none" };
	}
	// The Everyone group shares its id with the organization. Checked first so
	// it wins when the given group is Everyone itself.
	if (groupId === group.organization_id) {
		return { kind: "everyone" };
	}
	if (groupId === group.id) {
		return { kind: "this" };
	}
	return { kind: "other", groupId };
}

/** The AI budget cell: a member's spend against the viewed group's budget. */
const BudgetAmount: FC<{
	costControl: GroupMemberAICostControl;
	groupName: string;
	notAttributed: boolean;
	effectiveGroupName: string | undefined;
	isResolvingGroupName: boolean;
}> = ({
	costControl,
	groupName,
	notAttributed,
	effectiveGroupName,
	isResolvingGroupName,
}) => {
	const spend = costControl.current_spend_micros;

	// Also hides the spend when the governing group can't be resolved.
	if (notAttributed) {
		if (isResolvingGroupName) {
			return <Spinner loading size="sm" />;
		}
		if (!effectiveGroupName) {
			return (
				<LabelWithInfo
					label={emDash}
					message="This user's AI budget is managed by another org and isn't visible here."
				/>
			);
		}
		return (
			<div className="flex flex-col gap-0.5">
				<span className="flex items-center gap-1">
					<span>
						<span className="text-content-secondary">
							{formatBudgetUSD(spend)}
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

	const limit = costControl.spend_limit_micros;
	if (limit === null) {
		return (
			<LabelWithInfo
				label="Unlimited"
				message="None of this user's groups have an AI budget configured, so their AI usage isn't restricted."
			/>
		);
	}
	// A $0 budget disables spending, distinct from no budget configured.
	if (limit === 0) {
		return (
			<LabelWithInfo
				label="None"
				message="This user's group(s) have an AI budget of $0, so they have no AI spending allowance."
			/>
		);
	}

	return (
		<div className="flex flex-col gap-0.5">
			<span>
				<span className={severityAmountClassName(getSeverity(spend, limit))}>
					{formatBudgetUSD(spend)}
				</span>{" "}
				<span className="text-content-disabled">USD</span>
			</span>
			<span className="text-xs text-content-secondary">
				{`${costControl.limit_source === "user_override" ? "Custom" : "Group"} limit ${formatBudgetUSD(limit)}`}
			</span>
		</div>
	);
};

/** A label followed by an info tooltip. */
const LabelWithInfo: FC<{ label: ReactNode; message: ReactNode }> = ({
	label,
	message,
}) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<InfoIconTooltip message={message} />
	</span>
);
