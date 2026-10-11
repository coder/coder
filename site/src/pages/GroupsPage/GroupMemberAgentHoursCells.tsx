import { getErrorMessage } from "#/api/errors";
import type { AgentHoursGroupMemberUsage, Group } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Spinner } from "#/components/Spinner/Spinner";
import { TableCell, TableHead } from "#/components/Table/Table";
import { formatUsedAgentHours } from "#/utils/agentHours";
import { EM_DASH } from "./GroupMemberBudgetCells";
import { LabelWithInfo } from "./LabelWithInfo";

export const GroupMemberAgentHoursHeads: React.FC<{
	error: unknown;
}> = ({ error }) => (
	<>
		<TableHead>
			{error ? (
				<LabelWithInfo
					label="Agent Hours"
					kind="warning"
					message={getErrorMessage(error, "Unable to load Agent Hours.")}
					ariaLabel="Agent Hours couldn't be loaded"
				/>
			) : (
				<LabelWithInfo
					label="Agent Hours"
					message="Agent Hours this user used that counted toward this group in the current license usage period."
					ariaLabel="About Agent Hours"
				/>
			)}
		</TableHead>
		<TableHead>
			<LabelWithInfo
				label="Agent Hours group"
				message="A user's Agent Hours count toward their group with the largest Agent Hours allotment, or toward the organization's unallotted hours when none of their groups has one."
				ariaLabel="About Agent Hours groups"
			/>
		</TableHead>
	</>
);

/**
 * Hours are those that counted toward the viewed group; the group is where
 * the member's hours count now.
 */
export const GroupMemberAgentHoursCells: React.FC<{
	group: Group;
	username: string;
	usage: AgentHoursGroupMemberUsage | undefined;
	isLoading: boolean;
}> = ({ group, username, usage, isLoading }) => {
	if (isLoading) {
		return (
			<>
				<TableCell>
					<Spinner loading size="sm" />
				</TableCell>
				<TableCell>
					<Spinner loading size="sm" />
				</TableCell>
			</>
		);
	}
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
				{formatUsedAgentHours(usage.used_ms)}
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
						ariaLabel={`About ${username}'s Agent Hours`}
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

/** Everyone is checked first because a viewed Everyone group also matches "this". */
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
