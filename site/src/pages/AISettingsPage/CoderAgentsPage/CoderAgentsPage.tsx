import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import {
	chatAdvisorConfig,
	chatComputerUseProvider,
	chatPersonalModelOverridesAdminSettings,
	updateChatAdvisorConfig,
	updateChatComputerUseProvider,
	updateChatPersonalModelOverridesAdminSettings,
} from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAccessibleModelOrganizations } from "#/modules/aiModels/organizationModels";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import {
	modelOrganizationSearchParam,
	selectModelOrganization,
} from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { pageTitle } from "#/utils/page";
import { CoderAgentsPageView } from "./CoderAgentsPageView";
import { OrganizationAgentSettings } from "./OrganizationAgentSettings";

const CoderAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { experiments, organizations } = useDashboard();
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;
	const accessibleOrganizationsQuery =
		useAccessibleModelOrganizations(organizations);
	const organizationSelection = selectModelOrganization(
		accessibleOrganizationsQuery.organizations,
		searchParams.get(modelOrganizationSearchParam),
	);
	const activeOrganization = organizationSelection.organization;
	const organizationPermissionsQuery = useQuery({
		...organizationsPermissions(
			activeOrganization ? [activeOrganization.id] : undefined,
		),
		enabled: activeOrganization !== undefined,
	});
	const activeOrganizationPermissions = activeOrganization
		? organizationPermissionsQuery.data?.[activeOrganization.id]
		: undefined;
	const showAdvisorSettings = experiments.includes("chat-advisor");
	const showVirtualDesktopSettings = experiments.includes(
		"chat-virtual-desktop",
	);
	const personalOverridesQuery = useQuery({
		...chatPersonalModelOverridesAdminSettings(),
		enabled: canEditDeploymentConfig,
	});
	const advisorConfigQuery = useQuery({
		...chatAdvisorConfig(),
		enabled: canEditDeploymentConfig && showAdvisorSettings,
	});
	const computerUseProviderQuery = useQuery({
		...chatComputerUseProvider(),
		enabled: canEditDeploymentConfig && showVirtualDesktopSettings,
	});
	const savePersonalOverridesMutation = useMutation(
		updateChatPersonalModelOverridesAdminSettings(queryClient),
	);
	const saveAdvisorConfigMutation = useMutation(
		updateChatAdvisorConfig(queryClient),
	);
	const saveComputerUseProviderMutation = useMutation(
		updateChatComputerUseProvider(queryClient),
	);
	const canAccessOrganizationSettings =
		accessibleOrganizationsQuery.organizations.length > 0;
	const isFeatureVisible =
		canEditDeploymentConfig ||
		canAccessOrganizationSettings ||
		accessibleOrganizationsQuery.isLoading ||
		accessibleOrganizationsQuery.error !== null;
	const organizationSettings =
		activeOrganization &&
		organizationPermissionsQuery.data === undefined &&
		organizationPermissionsQuery.error == null ? (
			<Loader />
		) : activeOrganization &&
			organizationPermissionsQuery.data !== undefined ? (
			<OrganizationAgentSettings
				organization={activeOrganization}
				canEdit={
					!organizationSelection.requestedOrganizationDenied &&
					(activeOrganizationPermissions?.editChatModelConfigs ?? false)
				}
				showAdvisor={showAdvisorSettings}
			/>
		) : undefined;

	return (
		<RequirePermission isFeatureVisible={isFeatureVisible}>
			<title>{pageTitle("Coder Agents", "AI Settings")}</title>
			<CoderAgentsPageView
				organization={activeOrganization}
				organizations={accessibleOrganizationsQuery.organizations}
				onSelectOrganization={(organization) => {
					const next = new URLSearchParams(searchParams);
					next.set(modelOrganizationSearchParam, organization.name);
					setSearchParams(next);
				}}
				organizationAccessError={
					accessibleOrganizationsQuery.partialError ??
					accessibleOrganizationsQuery.error
				}
				organizationPermissionsError={organizationPermissionsQuery.error}
				requestedOrganizationDenied={
					organizationSelection.requestedOrganizationDenied
				}
				isOrganizationAccessLoading={accessibleOrganizationsQuery.isLoading}
				organizationSettings={organizationSettings}
				canEditDeploymentConfig={canEditDeploymentConfig}
				adminOverridesData={personalOverridesQuery.data}
				adminOverridesError={personalOverridesQuery.error}
				onRetryAdminOverrides={() => void personalOverridesQuery.refetch()}
				isRetryingAdminOverrides={personalOverridesQuery.isFetching}
				onSaveAdminOverrides={savePersonalOverridesMutation.mutate}
				isSavingAdminOverrides={savePersonalOverridesMutation.isPending}
				isSaveAdminOverridesError={savePersonalOverridesMutation.isError}
				showAdvisorSettings={showAdvisorSettings}
				advisorConfigData={advisorConfigQuery.data}
				isAdvisorConfigLoading={advisorConfigQuery.isLoading}
				isAdvisorConfigFetching={advisorConfigQuery.isFetching}
				isAdvisorConfigLoadError={advisorConfigQuery.isError}
				onSaveAdvisorConfig={saveAdvisorConfigMutation.mutate}
				isSavingAdvisorConfig={saveAdvisorConfigMutation.isPending}
				isSaveAdvisorConfigError={saveAdvisorConfigMutation.isError}
				saveAdvisorConfigError={saveAdvisorConfigMutation.error}
				showVirtualDesktopSettings={showVirtualDesktopSettings}
				computerUseProviderData={computerUseProviderQuery.data}
				isLoadingComputerUseProvider={computerUseProviderQuery.isLoading}
				onSaveComputerUseProvider={saveComputerUseProviderMutation.mutate}
				isSavingComputerUseProvider={saveComputerUseProviderMutation.isPending}
				computerUseProviderSaveError={saveComputerUseProviderMutation.error}
			/>
		</RequirePermission>
	);
};

export default CoderAgentsPage;
