import { type FC, type FormEvent, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { ModelSelector } from "#/modules/aiModels/ModelSelector";
import { ModelOverrideAlerts } from "#/pages/AgentsPage/components/ModelOverrideAlerts";
import {
	isUnavailableHistoricalModelID,
	type ProviderInfo,
	toEnabledModelSelectorOptions,
} from "#/pages/AgentsPage/utils/modelOptions";
import { AgentSettingLayout } from "./AgentSettingLayout";
import type { MutationCallbacks } from "./SubagentModelOverrideSettings";

type DefaultModelSettingsProps = {
	defaultModelID: string | undefined;
	enabledModels: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelsError: unknown;
	isLoading: boolean;
	onSaveDefaultModel: (modelID: string, options: MutationCallbacks) => void;
	isSaving: boolean;
	isSaveError: boolean;
	disabled: boolean;
};

export const DefaultModelSettings: FC<DefaultModelSettingsProps> = ({
	defaultModelID,
	enabledModels,
	providerInfoByID,
	modelsError,
	isLoading,
	onSaveDefaultModel,
	isSaving,
	isSaveError,
	disabled,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	// The unsaved selection is kept apart from the server value so a background
	// refetch of the model catalog cannot discard it before Save, which formik's
	// enableReinitialize would.
	const [pendingModelID, setPendingModelID] = useState<string>();
	const enabledModelOptions = toEnabledModelSelectorOptions(
		enabledModels,
		providerInfoByID,
	);
	// Discarded during render so a stale pick can neither be saved later nor
	// come back if the catalog relists the model.
	if (
		pendingModelID !== undefined &&
		(pendingModelID === defaultModelID ||
			!enabledModelOptions.some((option) => option.id === pendingModelID))
	) {
		setPendingModelID(undefined);
	}
	const selectedModelID = pendingModelID ?? defaultModelID;

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (pendingModelID !== undefined) {
			onSaveDefaultModel(pendingModelID, { onSuccess: showSavedState });
		}
	};
	const isFormDisabled = disabled || isSaving || isLoading;
	const canSave = !disabled && pendingModelID !== undefined;
	const isUnavailableSavedModel = isUnavailableHistoricalModelID(
		selectedModelID,
		enabledModelOptions,
	);

	return (
		<AgentSettingLayout
			title="Default model"
			description="Preselected for new chats and used when a chat's model is no longer available."
			showSave={canSave}
			isSaving={isSaving}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={handleSubmit}
			error={
				isSaveError ? (
					<p className="m-0">Failed to save default model.</p>
				) : undefined
			}
		>
			<div className="flex w-88 max-w-full flex-col gap-2">
				<ModelSelector
					options={enabledModelOptions}
					value={selectedModelID}
					onValueChange={setPendingModelID}
					triggerAriaLabel="Default model"
					disabled={isFormDisabled}
					placeholder={
						isLoading
							? "Loading models..."
							: isUnavailableSavedModel
								? "Unavailable model"
								: "Select model"
					}
					emptyMessage="No enabled models found."
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm"
					contentClassName="min-w-72"
				/>
				<ModelOverrideAlerts
					isUnavailableSavedModel={isUnavailableSavedModel}
					unavailableMessage="The default model is no longer enabled. Choose another model so new chats can start without picking one."
					modelsError={modelsError}
				/>
			</div>
		</AgentSettingLayout>
	);
};
