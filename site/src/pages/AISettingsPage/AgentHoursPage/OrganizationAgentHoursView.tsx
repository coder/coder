import type { AgentHoursGroupAllotments, Group } from "#/api/typesGenerated";
import { AllotmentPanel } from "./AllotmentPanel";
import {
	allotmentHours,
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
	const organizationBps = groupAllotments?.organization_allotment_bps;
	const organizationHours =
		organizationBps === undefined
			? undefined
			: allotmentHours(organizationBps, licenseHours);

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
					groupAllotments?.groups.map((allotment) => ({
						id: allotment.group_id,
						name: allotment.group_display_name || allotment.group_name,
						bps: allotment.allotment_bps,
					}))
				}
				candidates={(groups ?? [])
					.filter(
						(group) =>
							!groupAllotments?.groups.some(
								(allotment) => allotment.group_id === group.id,
							),
					)
					.map((group) => ({
						id: group.id,
						name: group.display_name || group.name,
					}))}
				poolHours={organizationHours}
				error={error}
				onSave={onSave}
				onRemove={onRemove}
			/>
		</div>
	);
};
