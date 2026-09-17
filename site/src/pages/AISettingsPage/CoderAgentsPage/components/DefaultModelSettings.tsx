import { useFormik } from "formik";
import type { FC } from "react";
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
	const hasLoadedDefault = defaultModelID !== undefined;
	const enabledModelOptions = toEnabledModelSelectorOptions(
		enabledModels,
		providerInfoByID,
	);

	const form = useFormik({
		enableReinitialize: true,
		initialValues: { model_config_id: defaultModelID ?? "" },
		onSubmit: (values, { resetForm }) => {
			onSaveDefaultModel(values.model_config_id, {
				onSuccess: () => {
					showSavedState();
					resetForm({ values });
				},
			});
		},
	});
	const isFormDisabled = disabled || isSaving || isLoading || !hasLoadedDefault;
	const canSave =
		hasLoadedDefault &&
		!disabled &&
		form.dirty &&
		form.values.model_config_id !== "";
	const isUnavailableSavedModel =
		form.values.model_config_id !== "" &&
		!enabledModelOptions.some(
			(option) => option.id === form.values.model_config_id,
		);

	return (
		<AgentSettingLayout
			title="Default model"
			description="Preselected for new chats and used by every context without an override."
			showSave={canSave}
			isSaving={isSaving}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				isSaveError ? (
					<p className="m-0">Failed to save default model.</p>
				) : undefined
			}
		>
			<div className="flex w-88 max-w-full flex-col gap-2">
				<ModelSelector
					options={enabledModelOptions}
					value={form.values.model_config_id}
					onValueChange={(value) =>
						void form.setFieldValue("model_config_id", value)
					}
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
