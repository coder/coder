import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	AdminPersonalModelOverridesSettings,
	type SavePersonalModelOverridesAdminSetting,
} from "./components/AdminPersonalModelOverridesSettings";
import {
	type MutationCallbacks,
	SubagentModelOverrideSettings,
} from "./components/SubagentModelOverrideSettings";

type SaveModelOverride = (
	req: { readonly model_config_id: string },
	options?: MutationCallbacks,
) => void;

export interface CoderAgentsPageViewProps {
	adminOverridesData?: TypesGen.ChatPersonalModelOverridesAdminSettings;
	adminOverridesError?: unknown;
	onRetryAdminOverrides?: () => void;
	isRetryingAdminOverrides?: boolean;
	onSaveAdminOverrides: SavePersonalModelOverridesAdminSetting;
	isSavingAdminOverrides: boolean;
	isSaveAdminOverridesError: boolean;
	generalModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	titleGenerationModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	exploreModelOverrideData?: TypesGen.ChatModelOverrideResponse;
	modelConfigsData: TypesGen.ChatModelConfig[] | undefined;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveGeneralModelOverride?: SaveModelOverride;
	isSavingGeneralModelOverride?: boolean;
	isSaveGeneralModelOverrideError?: boolean;
	onSaveTitleGenerationModel: SaveModelOverride;
	isSavingTitleGenerationModel: boolean;
	isSaveTitleGenerationModelError: boolean;
	onSaveExploreModelOverride: SaveModelOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
}

export const CoderAgentsPageView: FC<CoderAgentsPageViewProps> = ({
	adminOverridesData,
	adminOverridesError,
	onRetryAdminOverrides,
	isRetryingAdminOverrides,
	onSaveAdminOverrides,
	isSavingAdminOverrides,
	isSaveAdminOverridesError,
	generalModelOverrideData,
	titleGenerationModelOverrideData,
	exploreModelOverrideData,
	modelConfigsData,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride = false,
	isSaveGeneralModelOverrideError = false,
	onSaveTitleGenerationModel,
	isSavingTitleGenerationModel,
	isSaveTitleGenerationModelError,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
}) => {
	const enabledModelConfigs = (modelConfigsData ?? []).filter(
		(modelConfig) => modelConfig.enabled,
	);
	const showGeneralModelSection =
		onSaveGeneralModelOverride !== undefined ||
		generalModelOverrideData !== undefined ||
		isSavingGeneralModelOverride ||
		isSaveGeneralModelOverrideError;

	return (
		<div className="flex max-w-4xl flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Coder Agents</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Configure deployment-wide defaults for Coder Agents and agent-specific
					capabilities.
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
				{showGeneralModelSection && onSaveGeneralModelOverride && (
					<SubagentModelOverrideSettings
						title="General model"
						description="Used by delegated agents that can edit files or run commands."
						modelOverrideData={generalModelOverrideData}
						enabledModelConfigs={enabledModelConfigs}
						modelConfigsError={modelConfigsError}
						isLoading={isLoadingModelConfigs}
						onSaveModelOverride={onSaveGeneralModelOverride}
						isSaving={isSavingGeneralModelOverride}
						isSaveError={isSaveGeneralModelOverrideError}
						saveErrorMessage="Failed to save general model override."
					/>
				)}
				<SubagentModelOverrideSettings
					title="Title generation model"
					description="Leave unset to use Coder's title default, which prefers fast models from configured providers."
					modelOverrideData={titleGenerationModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveTitleGenerationModel}
					isSaving={isSavingTitleGenerationModel}
					isSaveError={isSaveTitleGenerationModelError}
					saveErrorMessage="Failed to save title generation model."
					unsetPlaceholder="Use title default"
					unavailableModelWarning="The selected model is currently unavailable. Title generation will be skipped until you choose another model or clear this setting."
				/>
				<SubagentModelOverrideSettings
					title="Explore subagent model"
					description="Used for read-only codebase exploration before work returns to the main agent."
					modelOverrideData={exploreModelOverrideData}
					enabledModelConfigs={enabledModelConfigs}
					modelConfigsError={modelConfigsError}
					isLoading={isLoadingModelConfigs}
					onSaveModelOverride={onSaveExploreModelOverride}
					isSaving={isSavingExploreModelOverride}
					isSaveError={isSaveExploreModelOverrideError}
					saveErrorMessage="Failed to save Explore model override."
				/>
			</div>
		</div>
	);
};
