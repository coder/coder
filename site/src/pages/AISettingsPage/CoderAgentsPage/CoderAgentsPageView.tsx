import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { AdvisorSettings } from "#/pages/AgentsPage/components/AdvisorSettings";
import { VirtualDesktopSettings } from "#/pages/AgentsPage/components/VirtualDesktopSettings";
import { OrganizationSettingsSection } from "#/pages/AISettingsPage/components/OrganizationSettingsSection";
import { SettingsSection } from "#/pages/AISettingsPage/components/SettingsSection";
import {
	AdminPersonalModelOverridesSettings,
	type SavePersonalModelOverridesAdminSetting,
} from "./components/AdminPersonalModelOverridesSettings";
import type { MutationCallbacks } from "./components/SubagentModelOverrideSettings";

export type CoderAgentsPageViewProps = {
	organization?: TypesGen.Organization;
	organizations: readonly TypesGen.Organization[];
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	organizationAccessError?: unknown;
	organizationPermissionsError?: unknown;
	requestedOrganizationDenied: boolean;
	isOrganizationAccessLoading: boolean;
	organizationSettings?: React.ReactNode;
	canEditDeploymentConfig: boolean;
	adminOverridesData?: TypesGen.ChatPersonalModelOverridesAdminSettings;
	adminOverridesError?: unknown;
	onRetryAdminOverrides?: () => void;
	isRetryingAdminOverrides?: boolean;
	onSaveAdminOverrides: SavePersonalModelOverridesAdminSetting;
	isSavingAdminOverrides: boolean;
	isSaveAdminOverridesError: boolean;
	showAdvisorSettings: boolean;
	advisorConfigData: TypesGen.AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	onSaveAdvisorConfig: (
		req: TypesGen.UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
	showVirtualDesktopSettings: boolean;
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	isLoadingComputerUseProvider: boolean;
	onSaveComputerUseProvider: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatComputerUseProviderRequest,
		unknown
	>;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
};

export const CoderAgentsPageView: React.FC<CoderAgentsPageViewProps> = ({
	organization,
	organizations,
	onSelectOrganization,
	organizationAccessError,
	organizationPermissionsError,
	requestedOrganizationDenied,
	isOrganizationAccessLoading,
	organizationSettings,
	canEditDeploymentConfig,
	adminOverridesData,
	adminOverridesError,
	onRetryAdminOverrides,
	isRetryingAdminOverrides,
	onSaveAdminOverrides,
	isSavingAdminOverrides,
	isSaveAdminOverridesError,
	showAdvisorSettings,
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
	showVirtualDesktopSettings,
	computerUseProviderData,
	isLoadingComputerUseProvider,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	return (
		<div className="flex max-w-4xl flex-col gap-10">
			<SettingsHeader>
				<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure organization model choices and deployment-wide Coder Agents
					capabilities.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{isOrganizationAccessLoading ? (
				<Loader label="Loading organization settings" />
			) : organization ? (
				<OrganizationSettingsSection
					title="Organization settings"
					description="Choose model and reasoning defaults for each Coder Agents context."
					organization={organization}
					organizations={organizations}
					onSelectOrganization={onSelectOrganization}
					requestedOrganizationDenied={requestedOrganizationDenied}
				>
					{organizationAccessError != null && (
						<ErrorAlert error={organizationAccessError} />
					)}
					{organizationPermissionsError != null && (
						<ErrorAlert error={organizationPermissionsError} />
					)}
					{organizationSettings}
				</OrganizationSettingsSection>
			) : organizationAccessError != null ? (
				<ErrorAlert error={organizationAccessError} />
			) : null}

			{canEditDeploymentConfig && (
				<SettingsSection
					title="Deployment settings"
					description="Configure Coder Agents capabilities that apply to every organization."
				>
					<div className="flex flex-col gap-6 rounded-lg border border-solid border-border px-6 py-7">
						<AdminPersonalModelOverridesSettings
							adminSettings={adminOverridesData}
							adminSettingsError={adminOverridesError}
							onRetryAdminSettings={onRetryAdminOverrides}
							isRetryingAdminSettings={isRetryingAdminOverrides}
							onSaveAdminSetting={onSaveAdminOverrides}
							isSavingAdminSetting={isSavingAdminOverrides}
							isSaveAdminSettingError={isSaveAdminOverridesError}
						/>
						{showVirtualDesktopSettings && (
							<VirtualDesktopSettings
								computerUseProviderData={computerUseProviderData}
								isLoadingComputerUseProvider={isLoadingComputerUseProvider}
								onSaveComputerUseProvider={onSaveComputerUseProvider}
								isSavingComputerUseProvider={isSavingComputerUseProvider}
								computerUseProviderSaveError={computerUseProviderSaveError}
							/>
						)}
						{showAdvisorSettings && (
							<AdvisorSettings
								advisorConfigData={advisorConfigData}
								isAdvisorConfigLoading={isAdvisorConfigLoading}
								isAdvisorConfigFetching={isAdvisorConfigFetching}
								isAdvisorConfigLoadError={isAdvisorConfigLoadError}
								onSaveAdvisorConfig={onSaveAdvisorConfig}
								isSavingAdvisorConfig={isSavingAdvisorConfig}
								isSaveAdvisorConfigError={isSaveAdvisorConfigError}
								saveAdvisorConfigError={saveAdvisorConfigError}
							/>
						)}
					</div>
				</SettingsSection>
			)}
		</div>
	);
};
