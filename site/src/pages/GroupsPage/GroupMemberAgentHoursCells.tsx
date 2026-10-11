import type { AgentHoursGroupMemberUsage, Group } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { TableCell } from "#/components/Table/Table";
import { LabelWithInfo } from "./LabelWithInfo";

const EM_DASH = "\u2014";

/**
 * The Agent hours and Agent Hours group cells for a group member. Hours are
 * those that counted toward the viewed group; the group is where the
 * member's hours count now.
 */
export const GroupMemberAgentHoursCells: React.FC<{
	group: Group;
	usage: AgentHoursGroupMemberUsage | undefined;
}> = ({ group, usage }) => {
	if (!usage) {
		return (
			<>
				<TableCell>{EM_DASH}</TableCell>
				<TableCell>{EM_DASH}</TableCell>
			</>
		);
	}

	const kind = agentHoursGroupKind(usage, group);
	const groupName = group.display_name || group.name;
	let effectiveGroupLabel: string;
	switch (kind) {
		case "everyone":
			effectiveGroupLabel = "Everyone (unallotted)";
			break;
		case "this":
			effectiveGroupLabel = groupName;
			break;
		case "otherGroup":
			effectiveGroupLabel =
				usage.effective_group.display_name || usage.effective_group.name;
			break;
	}

	const hours = (
		<span>
			<span className="text-content-primary">
				{formatUsedHours(usage.used_ms)}
			</span>{" "}
			<span className="text-content-secondary">hours</span>
		</span>
	);

	return (
		<>
			<TableCell className="whitespace-nowrap tabular-nums">
				{usage.effective_group.id === group.id ? (
					hours
				) : (
					<LabelWithInfo
						label={hours}
						message={`Only hours that counted toward ${groupName} are shown. This user's hours now count toward ${effectiveGroupLabel}.`}
					/>
				)}
			</TableCell>
			<TableCell>
				<Badge size="sm">
					{kind === "otherGroup"
						? `Counts toward ${effectiveGroupLabel}`
						: effectiveGroupLabel}
				</Badge>
			</TableCell>
		</>
	);
};

/** Floors to tenths so a partial hour never rounds up. */
const formatUsedHours = (usedMs: number): string =>
	(Math.floor(usedMs / 360_000) / 10).toLocaleString("en-US", {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	});

/**
 * Classifies the group a member's Agent Hours count toward, relative to the
 * viewed group:
 *
 * - "everyone": the organization's Everyone group, which stands for the
 *   organization's unallotted hours. Takes precedence over "this" when the
 *   viewed group is Everyone.
 * - "this": the viewed group.
 * - "otherGroup": another group in this organization.
 */
export function agentHoursGroupKind(
	usage: AgentHoursGroupMemberUsage,
	group: Pick<Group, "id" | "organization_id">,
): "everyone" | "this" | "otherGroup" {
	const groupId = usage.effective_group.id;
	if (groupId === group.organization_id) {
		return "everyone";
	}
	if (groupId === group.id) {
		return "this";
	}
	return "otherGroup";
}
