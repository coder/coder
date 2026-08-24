import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chatAdvisorConfig,
	chatComputerUseProvider,
	chatPersonalModelOverridesAdminSettings,
	updateChatAdvisorConfig,
	updateChatComputerUseProvider,
	updateChatPersonalModelOverridesAdminSettings,
} from "#/api/queries/chats";
import { entitlementDetails } from "#/api/queries/entitlements";
import { LicenseAgentRuntimeUsageUnavailableErrorText } from "#/api/typesGenerated";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { CoderAgentsPageView } from "./CoderAgentsPageView";

const CoderAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const { experiments } = useDashboard();
	const queryClient = useQueryClient();
	const canEditDeploymentConfig = permissions.editDeploymentConfig;
	const showAdvisorSettings = experiments.includes("chat-advisor");
	const showVirtualDesktopSettings = experiments.includes(
		"chat-virtual-desktop",
	);
	const entitlementDetailsQuery = useQuery(entitlementDetails());
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
	const agentRuntimeHoursFeature =
		entitlementDetailsQuery.data?.features.agent_runtime_hours;
	const hasAgentRuntimeLicense = entitlementDetailsQuery.data
		? agentRuntimeHoursFeature?.usage_period !== undefined
		: undefined;
	const isAgentRuntimeUsageUnavailable =
		entitlementDetailsQuery.data?.errors.includes(
			LicenseAgentRuntimeUsageUnavailableErrorText,
		) === true ||
		(entitlementDetailsQuery.data !== undefined &&
			agentRuntimeHoursFeature?.actual_ms === undefined);

	return (
		<RequirePermission isFeatureVisible={canEditDeploymentConfig}>
			<title>{pageTitle("Coder Agents", "AI Settings")}</title>
			<CoderAgentsPageView
				hasAgentRuntimeLicense={hasAgentRuntimeLicense}
				agentRuntimeHoursFeature={agentRuntimeHoursFeature}
				isAgentRuntimeUsageLoading={entitlementDetailsQuery.isLoading}
				isAgentRuntimeUsageUnavailable={isAgentRuntimeUsageUnavailable}
				agentRuntimeUsageError={entitlementDetailsQuery.error}
				onRetryAgentRuntimeUsage={() => void entitlementDetailsQuery.refetch()}
				isRetryingAgentRuntimeUsage={entitlementDetailsQuery.isFetching}
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
