import type {
	AgentHoursOrganizationAllotment,
	Organization,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { OrganizationSettingsSection } from "#/pages/AISettingsPage/components/OrganizationSettingsSection";
import { SettingsSection } from "#/pages/AISettingsPage/components/SettingsSection";
import { docs } from "#/utils/docs";
import { AllotmentPanel } from "./AllotmentPanel";
import { allotmentTargetLabel } from "./allotments";

type AgentHoursPageViewProps = {
	/** False when the license does not include Agent Hours. */
	isLicensed: boolean;
	canEditDeploymentConfig: boolean;
	/** False while organization access is unresolved for non-owners. */
	canManageAllotments: boolean;
	/** Licensed Agent Hours, undefined when unlimited. */
	licenseHours: number | undefined;
	organizationAllotments:
		| readonly AgentHoursOrganizationAllotment[]
		| undefined;
	organizationAllotmentsError: unknown;
	/** Every organization, for picking a new organization allotment. */
	organizations: readonly Organization[];
	onSaveOrganizationAllotment: (
		organizationId: string,
		bps: number,
	) => Promise<unknown>;
	onRemoveOrganizationAllotment: (organizationId: string) => Promise<unknown>;
	organization: Organization | undefined;
	/** Organizations whose group allotments the user can manage. */
	groupAllotmentOrganizations: readonly Organization[];
	onSelectOrganization: (organization: Organization) => void;
	requestedOrganizationDenied: boolean;
	isOrganizationAccessLoading: boolean;
	organizationAccessError: unknown;
	organizationAgentHours: React.ReactNode;
};

export const AgentHoursPageView: React.FC<AgentHoursPageViewProps> = ({
	isLicensed,
	canEditDeploymentConfig,
	canManageAllotments,
	licenseHours,
	organizationAllotments,
	organizationAllotmentsError,
	organizations,
	onSaveOrganizationAllotment,
	onRemoveOrganizationAllotment,
	organization,
	groupAllotmentOrganizations,
	onSelectOrganization,
	requestedOrganizationDenied,
	isOrganizationAccessLoading,
	organizationAccessError,
	organizationAgentHours,
}) => {
	const showLicenseNotice = !isLicensed && canManageAllotments;
	const allottedOrganizations = organizationAllotments?.map((allotment) => ({
		id: allotment.organization_id,
		name: allotment.organization_name,
		display_name: allotment.organization_display_name,
		bps: allotment.allotment_bps,
	}));
	const organizationTargets = [
		...(allottedOrganizations ?? []),
		...organizations,
	];

	return (
		<div className="flex max-w-4xl flex-col gap-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Agent Hours</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Divide your deployment's licensed Agent Hours between organizations
					and groups. Coder does not enforce allotments yet.{" "}
					<SettingsHeaderDocsLink
						href={docs(
							"/ai-coder/agents/platform-controls/agent-hours-allotments",
						)}
					/>
				</SettingsHeaderDescription>
			</SettingsHeader>

			{showLicenseNotice && (
				<Alert severity="info">
					<AlertTitle>Your license does not include Agent Hours</AlertTitle>
					<AlertDescription>
						Allotments need a Premium license that grants Agent Hours.
					</AlertDescription>
				</Alert>
			)}

			{isLicensed && canEditDeploymentConfig && (
				<SettingsSection
					title="Organization allotments"
					description={
						licenseHours === undefined
							? "Shares of the deployment's Agent Hours. Your license grants unlimited Agent Hours, so allotments are shown as percentages. Organizations without an allotment draw from the unallotted shared pool."
							: "Shares of the deployment's Agent Hours. Organizations without an allotment draw from the unallotted shared pool."
					}
				>
					<AllotmentPanel
						entity="organization"
						poolLabel="the deployment's Agent Hours"
						allotments={allottedOrganizations?.map((allotted) => ({
							id: allotted.id,
							name: allotmentTargetLabel(allotted, organizationTargets),
							bps: allotted.bps,
						}))}
						candidates={organizations
							.filter(
								(candidate) =>
									!allottedOrganizations?.some(
										(allotted) => allotted.id === candidate.id,
									),
							)
							.map((candidate) => ({
								id: candidate.id,
								name: allotmentTargetLabel(candidate, organizationTargets),
							}))}
						poolHours={licenseHours}
						error={organizationAllotmentsError}
						onSave={onSaveOrganizationAllotment}
						onRemove={onRemoveOrganizationAllotment}
					/>
				</SettingsSection>
			)}

			{!showLicenseNotice && isOrganizationAccessLoading && (
				<Loader label="Loading organizations" />
			)}
			{!showLicenseNotice && organizationAccessError != null && (
				<ErrorAlert error={organizationAccessError} />
			)}
			{isLicensed && organization && (
				<OrganizationSettingsSection
					title="Group allotments"
					description="Shares of this organization's Agent Hours."
					organization={organization}
					organizations={groupAllotmentOrganizations}
					onSelectOrganization={onSelectOrganization}
					requestedOrganizationDenied={requestedOrganizationDenied}
				>
					{organizationAgentHours}
				</OrganizationSettingsSection>
			)}
		</div>
	);
};
