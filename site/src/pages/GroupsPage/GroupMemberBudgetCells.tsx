import type { FC, ReactNode } from "react";
import { useQuery } from "react-query";
import type { GroupMemberAICostControl } from "#/api/api";
import { groupById } from "#/api/queries/groups";
import type { Group } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell } from "#/components/Table/Table";
import { getSeverity, severityAmountClassName } from "#/utils/budget";
import { formatBudgetUSD } from "#/utils/currency";
import { InfoIconTooltip } from "./InfoIconTooltip";

// Escaped so the emdash lint doesn't flag a literal.
const emDash = "\u2014";

/**
 * The AI budget and Budget group cells for a group member. The page only
 * reports spend against the viewed group; when another group governs the
 * member's budget, that spend isn't attributed here. Both cells share the same
 * derivation, so they're built together.
 */
export const GroupMemberBudgetCells: FC<{
	group: Group;
	userID: string;
	costControl: GroupMemberAICostControl | undefined;
}> = ({ group, userID, costControl }) => {
	const effectiveGroupID = costControl?.effective_group_id ?? null;
	const effectiveIsThisGroup = effectiveGroupID === group.id;
	// The everyone group shares its id with the organization.
	const effectiveIsEveryone = effectiveGroupID === group.organization_id;
	// Another group governs the budget, so spend here isn't counted against it.
	const notAttributed = !effectiveIsThisGroup && !effectiveIsEveryone;

	// Resolve the governing group's name only when it's another named group.
	const needsName = Boolean(effectiveGroupID) && notAttributed;
	const { data: effectiveGroup } = useQuery({
		...groupById(effectiveGroupID ?? "", { exclude_members: true }),
		enabled: needsName,
	});
	const effectiveGroupName =
		effectiveGroup?.display_name || effectiveGroup?.name;
	const groupName = group.display_name || group.name;

	let budgetGroup: ReactNode = emDash;
	if (costControl) {
		if (effectiveIsEveryone) {
			budgetGroup = <Badge size="sm">Everyone (not allocated)</Badge>;
		} else if (effectiveGroupID !== null) {
			const name = effectiveIsThisGroup ? groupName : effectiveGroupName;
			if (name) {
				budgetGroup = (
					<Badge size="sm">
						{costControl.limit_source === "override"
							? `${name} (individual)`
							: name}
					</Badge>
				);
			}
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
					/>
				) : (
					emDash
				)}
			</TableCell>
			<TableCell>{budgetGroup}</TableCell>
		</>
	);
};

/** The AI budget cell: a member's spend against the viewed group's budget. */
const BudgetAmount: FC<{
	costControl: GroupMemberAICostControl;
	groupName: string;
	notAttributed: boolean;
	effectiveGroupName: string | undefined;
}> = ({ costControl, groupName, notAttributed, effectiveGroupName }) => {
	const spend = costControl.current_spend_micros;

	// Governed by another group: the spend happened here but isn't counted
	// against this group's budget.
	if (notAttributed) {
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
							effectiveGroupName ? (
								<>
									This spend happened in the{" "}
									<span className="font-medium text-content-primary">
										{groupName}
									</span>{" "}
									group, but this user's AI budget is managed by the{" "}
									<span className="font-medium text-content-primary">
										{effectiveGroupName}
									</span>{" "}
									group, so it isn't counted here.
								</>
							) : (
								"This spend happened in this group, but this user's AI budget is managed by another group, so it isn't counted here."
							)
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

	const sub = `${costControl.limit_source === "override" ? "Custom" : "Group"} limit ${formatBudgetUSD(limit)}`;
	return (
		<div className="flex flex-col gap-0.5">
			<span>
				<span className={severityAmountClassName(getSeverity(spend, limit))}>
					{formatBudgetUSD(spend)}
				</span>{" "}
				<span className="text-content-disabled">USD</span>
			</span>
			<span className="text-xs text-content-secondary">{sub}</span>
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
