import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
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
}) => (
	<div className="flex max-w-4xl flex-col gap-8">
		<SettingsHeader>
			<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
			<SettingsHeaderDescription>
				Configure deployment-wide Coder Agents capabilities. Model defaults are
				configured per organization in{" "}
				<Link to="/ai/settings/models/defaults">Defaults & overrides</Link>.
			</SettingsHeaderDescription>
		</SettingsHeader>
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
	</div>
);
