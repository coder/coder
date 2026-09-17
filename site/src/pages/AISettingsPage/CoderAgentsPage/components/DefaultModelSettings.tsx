import { type FC, type FormEvent, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { ModelSelector } from "#/modules/aiModels/ModelSelector";
import { ModelOverrideAlerts } from "#/pages/AgentsPage/components/ModelOverrideAlerts";
import {
	type ProviderInfo,
	toEnabledModelSelectorOptions,
} from "#/pages/AgentsPage/utils/modelOptions";
import { AgentSettingLayout } from "./AgentSettingLayout";
import type { MutationCallbacks } from "./SubagentModelOverrideSettings";

interface DefaultModelSettingsProps {
	/** Undefined while the model catalog is loading or failed; "" when the organization has no default. */
	defaultModelID: string | undefined;
	enabledModels: readonly TypesGen.ChatModel[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelsError: unknown;
	isLoading: boolean;
	onSaveDefaultModel: (modelId: string, options?: MutationCallbacks) => void;
	isSaving: boolean;
	isSaveError: boolean;
	disabled?: boolean;
}

export const DefaultModelSettings: FC<DefaultModelSettingsProps> = ({
	defaultModelID,
	enabledModels,
	providerInfoByID,
	modelsError,
	isLoading,
	onSaveDefaultModel,
	isSaving,
	isSaveError,
	disabled = false,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	// The unsaved selection is kept apart from the server value so a background
	// refetch of the model catalog cannot discard it before Save. A pending
	// model that the refetch no longer lists is dropped instead of being saved.
	const [pendingModelID, setPendingModelID] = useState<string>();
	const hasLoadedDefault = defaultModelID !== undefined;
	const savedModelID = defaultModelID ?? "";
	const enabledModelOptions = toEnabledModelSelectorOptions(
		enabledModels,
		providerInfoByID,
	);
	const selectedModelID =
		pendingModelID !== undefined &&
		enabledModelOptions.some((option) => option.id === pendingModelID)
			? pendingModelID
			: savedModelID;

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		onSaveDefaultModel(selectedModelID, {
			onSuccess: () => {
				showSavedState();
				setPendingModelID(undefined);
			},
		});
	};
	const isFormDisabled = disabled || isSaving || isLoading || !hasLoadedDefault;
	const canSave =
		hasLoadedDefault && !disabled && selectedModelID !== savedModelID;
	const isUnavailableSavedModel =
		selectedModelID !== "" &&
		!enabledModelOptions.some((option) => option.id === selectedModelID);

	return (
		<AgentSettingLayout
			title="Default model"
			description="Preselected for new chats and used by every context without an override."
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
					onValueChange={(value) =>
						setPendingModelID(value === savedModelID ? undefined : value)
					}
					triggerAriaLabel="Default model"
					disabled={isFormDisabled}
					placeholder={
						isUnavailableSavedModel ? "Unavailable model" : "Select a model"
					}
					emptyMessage={
						isLoading ? "Loading models..." : "No enabled models found."
					}
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm"
					contentClassName="min-w-[18rem]"
				/>
				<ModelOverrideAlerts
					isUnavailableSavedModel={isUnavailableSavedModel}
					unavailableMessage="The default model is currently unavailable. Choose another model so new chats can start without picking one."
					modelsError={modelsError}
				/>
			</div>
		</AgentSettingLayout>
	);
};
