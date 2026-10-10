import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import {
	agentHoursAllotmentOrganizations,
	agentHoursOrganizationAllotments,
	deleteAgentHoursOrganizationAllotment,
	upsertAgentHoursOrganizationAllotment,
} from "#/api/queries/agentHours";
import { organizations } from "#/api/queries/organizations";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { AgentHoursPageView } from "./AgentHoursPageView";
import { canManageAgentHoursAllotments } from "./agentHoursAccess";
import { OrganizationAgentHours } from "./OrganizationAgentHours";

const AgentHoursPage: React.FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const feature = entitlements.features.agent_runtime_hours;
	const licenseHours = feature.limit;

	// Loaded without the license too, so everyone who could manage allotments
	// sees the license notice instead of a permission denial. Refetched on
	// focus because roles can be revoked while the page is open.
	const allotmentOrganizationsQuery = useQuery({
		...agentHoursAllotmentOrganizations(),
		refetchOnWindowFocus: true,
	});
	const allotmentOrganizations = allotmentOrganizationsQuery.data ?? [];
	const organizationSelection = selectModelOrganization(
		allotmentOrganizations,
		searchParams.get(modelOrganizationSearchParam),
	);
	const activeOrganization = organizationSelection.organization;

	const organizationAllotmentsQuery = useQuery({
		...agentHoursOrganizationAllotments(),
		enabled: feature.enabled && permissions.editDeploymentConfig,
	});
	// Organizations deleted elsewhere must leave the Add candidates, as
	// deleted groups do.
	const organizationsQuery = useQuery({
		...organizations(),
		enabled: feature.enabled && permissions.editDeploymentConfig,
		refetchOnWindowFocus: true,
	});
	// Do not deny access before organization access resolves; the view shows a
	// failed lookup's error instead.
	const isAccessPending =
		allotmentOrganizationsQuery.isLoading ||
		allotmentOrganizationsQuery.error != null;
	const upsertMutation = useMutation(
		upsertAgentHoursOrganizationAllotment(queryClient),
	);
	const deleteMutation = useMutation(
		deleteAgentHoursOrganizationAllotment(queryClient),
	);
	const canManageAllotments = canManageAgentHoursAllotments(
		permissions,
		allotmentOrganizationsQuery.data,
	);

	return (
		<RequirePermission
			isFeatureVisible={isAccessPending || canManageAllotments}
		>
			<title>{pageTitle("Agent Hours", "AI Settings")}</title>

			<AgentHoursPageView
				isLicensed={feature.enabled}
				canEditDeploymentConfig={permissions.editDeploymentConfig}
				canManageAllotments={canManageAllotments}
				licenseHours={licenseHours}
				organizationAllotments={organizationAllotmentsQuery.data}
				organizationAllotmentsError={organizationAllotmentsQuery.error}
				organizations={organizationsQuery.data ?? []}
				onSaveOrganizationAllotment={(organizationId, allotmentBps) =>
					upsertMutation.mutateAsync({ organizationId, allotmentBps })
				}
				onRemoveOrganizationAllotment={deleteMutation.mutateAsync}
				organization={activeOrganization}
				groupAllotmentOrganizations={allotmentOrganizations}
				onSelectOrganization={(organization) => {
					const next = new URLSearchParams(searchParams);
					next.set(modelOrganizationSearchParam, organization.name);
					setSearchParams(next);
				}}
				requestedOrganizationDenied={
					organizationSelection.requestedOrganizationDenied
				}
				isOrganizationAccessLoading={allotmentOrganizationsQuery.isLoading}
				organizationAccessError={allotmentOrganizationsQuery.error}
				organizationAgentHours={
					activeOrganization && (
						<OrganizationAgentHours
							key={activeOrganization.id}
							organization={activeOrganization}
							licenseHours={licenseHours}
						/>
					)
				}
			/>
		</RequirePermission>
	);
};

export default AgentHoursPage;
