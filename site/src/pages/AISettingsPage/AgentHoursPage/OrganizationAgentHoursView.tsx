import type { AgentHoursGroupAllotments, Group } from "#/api/typesGenerated";
import { isEveryoneGroup } from "#/modules/groups";
import { AllotmentPanel } from "./AllotmentPanel";
import {
	allotmentHours,
	allotmentTargetLabel,
	formatAllotmentPercent,
	formatHours,
} from "./allotments";

type OrganizationAgentHoursViewProps = {
	/** Licensed Agent Hours, undefined when unlimited. */
	licenseHours: number | undefined;
	groupAllotments: AgentHoursGroupAllotments | undefined;
	groups: readonly Group[] | undefined;
	error: unknown;
	onSave: (groupId: string, bps: number) => Promise<unknown>;
	onRemove: (groupId: string) => Promise<unknown>;
};

export const OrganizationAgentHoursView: React.FC<
	OrganizationAgentHoursViewProps
> = ({ licenseHours, groupAllotments, groups, error, onSave, onRemove }) => {
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
	const groupTargets = [...(allottedGroups ?? []), ...(groups ?? [])];

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
				onSave={onSave}
				onRemove={onRemove}
			/>
		</div>
	);
};
