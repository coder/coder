import type {
	AgentHoursGroupAllotments,
	AgentHoursOrganizationGroupsUsage,
	Group,
	Organization,
} from "#/api/typesGenerated";
import { isEveryoneGroup } from "#/modules/groups";
import { AllotmentPanel, type AllotmentUsage } from "./AllotmentPanel";
import {
	allotmentHours,
	allotmentTargetLabel,
	formatAllotmentPercent,
	formatHours,
} from "./allotments";

type OrganizationAgentHoursViewProps = {
	organization: Organization;
	/** Licensed Agent Hours, undefined when unlimited. */
	licenseHours: number | undefined;
	groupAllotments: AgentHoursGroupAllotments | undefined;
	groups: readonly Group[] | undefined;
	error: unknown;
	usage: AgentHoursOrganizationGroupsUsage | undefined;
	usageError: unknown;
	onSave: (groupId: string, bps: number) => Promise<unknown>;
	onRemove: (groupId: string) => Promise<unknown>;
};

export const OrganizationAgentHoursView: React.FC<
	OrganizationAgentHoursViewProps
> = ({
	organization,
	licenseHours,
	groupAllotments,
	groups,
	error,
	usage,
	usageError,
	onSave,
	onRemove,
}) => {
	const organizationBps =
		groupAllotments?.organization_allotment_bps ?? undefined;
	const organizationHours =
		organizationBps === undefined
			? undefined
			: allotmentHours(organizationBps, licenseHours);
	const allottedGroups = groupAllotments?.groups.map((allotment) => ({
		id: allotment.group_id,
		name: allotment.group_name,
		display_name: allotment.group_display_name,
		bps: allotment.allotment_bps,
	}));
	const usedGroups = usage?.groups.map((group) => ({
		id: group.group_id,
		name: group.group_name,
		display_name: group.group_display_name,
		usedMs: group.used_ms,
	}));
	const groupTargets = [
		...(allottedGroups ?? []),
		...(groups ?? []),
		...(usedGroups ?? []),
	];
	const groupHref = (groupName: string) =>
		`/organizations/${organization.name}/groups/${groupName}`;
	const everyoneGroup = groups?.find(isEveryoneGroup);
	const everyoneUsage = usedGroups?.find(
		(group) => group.id === organization.id,
	);
	const groupUsage: AllotmentUsage | undefined = usedGroups && {
		unallotted: usedGroups
			.filter(
				(used) =>
					used.id !== organization.id &&
					used.usedMs > 0 &&
					!allottedGroups?.some((allotted) => allotted.id === used.id),
			)
			.map((used) =>
				used.name
					? {
							id: used.id,
							name: allotmentTargetLabel(used, groupTargets),
							usedMs: used.usedMs,
							href: groupHref(used.name),
						}
					: { id: used.id, name: "Deleted group", usedMs: used.usedMs },
			),
		remainderLabel: "Everyone else (unallotted)",
		remainderHref: everyoneGroup && groupHref(everyoneGroup.name),
		remainderUsedMs: everyoneUsage?.usedMs ?? 0,
	};

	return (
		<div className="flex flex-col gap-4">
			{groupAllotments && (
				<p className="m-0 text-sm text-content-secondary">
					{organizationBps === undefined ? (
						<>
							<span className="font-medium text-content-primary">
								No allotment
							</span>
							; draws from the shared pool. Group allotments are shown as
							percentages until this organization has an allotment.
						</>
					) : (
						<>
							<span className="font-medium text-content-primary">
								{formatAllotmentPercent(organizationBps)} of Agent Hours
								{organizationHours !== undefined &&
									` (${formatHours(organizationHours)})`}
							</span>
							{organizationHours === undefined &&
								". Agent Hours are unlimited, so group allotments are shown as percentages."}
						</>
					)}
				</p>
			)}
			<AllotmentPanel
				entity="group"
				poolLabel="this organization's Agent Hours"
				allotments={
					groups &&
					allottedGroups?.map((allotted) => ({
						id: allotted.id,
						name: allotmentTargetLabel(allotted, groupTargets),
						bps: allotted.bps,
						usedMs: usedGroups?.find((used) => used.id === allotted.id)?.usedMs,
						href: groupHref(allotted.name),
					}))
				}
				candidates={(groups ?? [])
					.filter(
						(group) =>
							!isEveryoneGroup(group) &&
							!allottedGroups?.some((allotted) => allotted.id === group.id),
					)
					.map((group) => ({
						id: group.id,
						name: allotmentTargetLabel(group, groupTargets),
					}))}
				poolHours={organizationHours}
				error={error}
				usage={groupUsage}
				usageError={usageError}
				onSave={onSave}
				onRemove={onRemove}
			/>
		</div>
	);
};
