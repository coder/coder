import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import type { ModelSelectorOption } from "./components/ChatElements";
import {
	PersonalModelOverrideRow,
	type SavePersonalOverride,
} from "./components/PersonalModelOverrideRow";
import { SectionHeader } from "./components/SectionHeader";

export interface AgentSettingsUserAgentsPageViewProps {
	overridesData?: TypesGen.UserChatPersonalModelOverridesResponse;
	overridesError: unknown;
	onRetryOverrides?: () => void;
	isRetryingOverrides?: boolean;
	isLoadingOverrides: boolean;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError: unknown;
	isLoadingModels: boolean;
	onSaveRootModelOverride: SavePersonalOverride;
	isSavingRootModelOverride: boolean;
	isSaveRootModelOverrideError: boolean;
	onSaveGeneralModelOverride: SavePersonalOverride;
	isSavingGeneralModelOverride: boolean;
	isSaveGeneralModelOverrideError: boolean;
	onSaveExploreModelOverride: SavePersonalOverride;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
}

export const AgentSettingsUserAgentsPageView: FC<
	AgentSettingsUserAgentsPageViewProps
> = ({
	overridesData,
	overridesError,
	onRetryOverrides,
	isRetryingOverrides = false,
	isLoadingOverrides,
	modelOptions,
	modelConfigs,
	modelConfigsError,
	isLoadingModels,
	onSaveRootModelOverride,
	isSavingRootModelOverride,
	isSaveRootModelOverrideError,
	onSaveGeneralModelOverride,
	isSavingGeneralModelOverride,
	isSaveGeneralModelOverrideError,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
}) => {
	const personalOverridesEnabled = overridesData?.enabled ?? true;
	const isLoading = isLoadingOverrides || isLoadingModels;
	const isDisabled = isLoading || !personalOverridesEnabled;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Agents"
				description="Choose personal model defaults for root agents and delegated agents."
			/>
			{overridesError ? (
				<div className="flex flex-col gap-2">
					<ErrorAlert error={overridesError} />
					{onRetryOverrides && (
						<Button
							disabled={isRetryingOverrides}
							onClick={onRetryOverrides}
							size="sm"
							type="button"
							variant="outline"
						>
							Retry
						</Button>
					)}
				</div>
			) : null}
			{!personalOverridesEnabled && (
				<Alert severity="info">
					<AlertDescription>
						Personal model overrides are disabled by an administrator. Saved
						values are shown for reference, but changes cannot be saved.
					</AlertDescription>
				</Alert>
			)}
			<PersonalModelOverrideRow
				context="root"
				title="Root agent model"
				description="Choose the model behavior for new root agents."
				overrideData={overridesData?.root}
				modelOptions={modelOptions}
				modelConfigs={modelConfigs}
				modelConfigsError={modelConfigsError}
				isLoading={isLoading}
				onSave={onSaveRootModelOverride}
				isSaving={isSavingRootModelOverride}
				isSaveError={isSaveRootModelOverrideError}
				saveErrorMessage="Failed to save root agent model override."
				disabled={isDisabled}
			/>
			<PersonalModelOverrideRow
				context="general"
				title="General subagent model"
				description="Choose the model behavior for delegated agents with write capabilities."
				overrideData={overridesData?.general}
				deploymentDefault={overridesData?.deployment_defaults.general}
				modelOptions={modelOptions}
				modelConfigs={modelConfigs}
				modelConfigsError={modelConfigsError}
				isLoading={isLoading}
				onSave={onSaveGeneralModelOverride}
				isSaving={isSavingGeneralModelOverride}
				isSaveError={isSaveGeneralModelOverrideError}
				saveErrorMessage="Failed to save general subagent model override."
				disabled={isDisabled}
			/>
			<PersonalModelOverrideRow
				context="explore"
				title="Explore subagent model"
				description="Choose the model behavior for read-only Explore subagents."
				overrideData={overridesData?.explore}
				deploymentDefault={overridesData?.deployment_defaults.explore}
				modelOptions={modelOptions}
				modelConfigs={modelConfigs}
				modelConfigsError={modelConfigsError}
				isLoading={isLoading}
				onSave={onSaveExploreModelOverride}
				isSaving={isSavingExploreModelOverride}
				isSaveError={isSaveExploreModelOverrideError}
				saveErrorMessage="Failed to save Explore subagent model override."
				disabled={isDisabled}
			/>
		</div>
	);
};
