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

/** True when a named group other than Everyone governs this member's budget. */
export function isBudgetFromOtherGroup(
	costControl: GroupMemberAICostControl | undefined,
	group: Pick<Group, "id" | "organization_id">,
): boolean {
	const effectiveGroupID = costControl?.effective_group_id ?? null;
	return (
		effectiveGroupID !== null &&
		effectiveGroupID !== group.id &&
		effectiveGroupID !== group.organization_id
	);
}

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
	// Unlike isBudgetFromOtherGroup, this also covers a null effective group
	// (nothing resolves), which the cell must still render as unattributed.
	const notAttributed = !effectiveIsThisGroup && !effectiveIsEveryone;

	// Resolve the governing group's name only when it's another named group.
	const { data: effectiveGroup, isLoading: isResolvingGroupName } = useQuery({
		...groupById(effectiveGroupID ?? "", { exclude_members: true }),
		enabled: Boolean(effectiveGroupID) && notAttributed,
	});
	const effectiveGroupName =
		effectiveGroup?.display_name || effectiveGroup?.name;
	const groupName = group.display_name || group.name;

	let budgetGroup: ReactNode = emDash;
	if (costControl) {
		if (effectiveIsEveryone) {
			budgetGroup = <Badge size="sm">Everyone (not allocated)</Badge>;
		} else if (effectiveGroupID !== null) {
			// "Another org" when the governing group can't be resolved.
			const name = effectiveIsThisGroup ? groupName : effectiveGroupName;
			budgetGroup = (
				<Badge size="sm">
					{name
						? costControl.limit_source === "user_override"
							? `${name} (individual)`
							: name
						: "Another org"}
				</Badge>
			);
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

	// Governed by another group. If it can't be resolved (e.g. another org),
	// the spend isn't shown either.
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
