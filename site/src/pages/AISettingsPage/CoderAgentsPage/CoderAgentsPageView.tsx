import { type FC, type ReactNode, useId } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { AdvisorSettings } from "#/pages/AgentsPage/components/AdvisorSettings";
import { VirtualDesktopSettings } from "#/pages/AgentsPage/components/VirtualDesktopSettings";
import {
	AdminPersonalModelOverridesSettings,
	type SavePersonalModelOverridesAdminSetting,
} from "./components/AdminPersonalModelOverridesSettings";
import type { MutationCallbacks } from "./components/SubagentModelOverrideSettings";

export interface CoderAgentsPageViewProps {
	organization?: TypesGen.Organization;
	organizations: readonly TypesGen.Organization[];
	onSelectOrganization: (organization: TypesGen.Organization) => void;
	organizationAccessError?: unknown;
	organizationPermissionsError?: unknown;
	requestedOrganizationDenied: boolean;
	isOrganizationAccessLoading: boolean;
	organizationSettings?: ReactNode;
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
}

export const CoderAgentsPageView: FC<CoderAgentsPageViewProps> = ({
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
	const organizationSettingsHeadingId = useId();
	const deploymentSettingsHeadingId = useId();

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
				<section
					aria-labelledby={organizationSettingsHeadingId}
					className="flex flex-col gap-6"
				>
					<div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
						<div>
							<h2
								id={organizationSettingsHeadingId}
								className="m-0 text-xl font-semibold"
							>
								Organization settings
							</h2>
							<p className="mt-1 mb-0 text-sm text-content-secondary">
								Choose model and reasoning defaults for each Coder Agents
								context.
							</p>
						</div>
						{organizations.length > 1 && (
							<OrganizationAutocomplete
								value={organization}
								ariaLabel={`Organization ${getOrganizationLabel(
									organization,
									organizations,
								)}`}
								options={organizations}
								triggerClassName="w-60"
								optionsTabbable
								onChange={(nextOrganization) => {
									if (nextOrganization) {
										onSelectOrganization(nextOrganization);
									}
								}}
							/>
						)}
					</div>
					{requestedOrganizationDenied && (
						<Alert severity="warning">
							The requested organization is not available. Showing settings for{" "}
							{organization.display_name || organization.name} instead.
						</Alert>
					)}
					{organizationAccessError != null && (
						<ErrorAlert error={organizationAccessError} />
					)}
					{organizationPermissionsError != null && (
						<ErrorAlert error={organizationPermissionsError} />
					)}
					{organizationSettings}
				</section>
			) : organizationAccessError != null ? (
				<ErrorAlert error={organizationAccessError} />
			) : null}

			{canEditDeploymentConfig && (
				<section
					aria-labelledby={deploymentSettingsHeadingId}
					className="flex flex-col gap-6"
				>
					<div>
						<h2
							id={deploymentSettingsHeadingId}
							className="m-0 text-xl font-semibold"
						>
							Deployment settings
						</h2>
						<p className="mt-1 mb-0 text-sm text-content-secondary">
							Configure Coder Agents capabilities that apply to every
							organization.
						</p>
					</div>
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
				</section>
			)}
		</div>
	);
};
