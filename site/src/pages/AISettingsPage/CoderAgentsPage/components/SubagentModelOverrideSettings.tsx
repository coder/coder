import { useFormik } from "formik";
import type { FC, ReactNode } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { ModelSelector } from "#/pages/AgentsPage/components/ChatElements/ModelSelector";
import { ModelOverrideAlerts } from "#/pages/AgentsPage/components/ModelOverrideAlerts";
import type { ProviderInfo } from "#/pages/AgentsPage/utils/modelOptions";
import { AgentSettingLayout } from "./AgentSettingLayout";

export interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface ModelOverrideData {
	readonly model_config_id: string;
	readonly is_malformed: boolean;
}

interface UpdateModelOverrideRequest {
	readonly model_config_id: string;
}

interface SubagentModelOverrideSettingsProps {
	title: string;
	description?: ReactNode;
	modelOverrideData: ModelOverrideData | undefined;
	enabledModelConfigs: readonly TypesGen.ChatModelConfig[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelConfigsError: unknown;
	isLoading: boolean;
	onSaveModelOverride: (
		req: UpdateModelOverrideRequest,
		options?: MutationCallbacks,
	) => void;
	isSaving: boolean;
	isSaveError: boolean;
	saveErrorMessage: string;
	unsetPlaceholder?: string;
	unavailableModelWarning?: string;
	disabled?: boolean;
}

export const SubagentModelOverrideSettings: FC<
	SubagentModelOverrideSettingsProps
> = ({
	title,
	description,
	modelOverrideData,
	enabledModelConfigs,
	providerInfoByID,
	modelConfigsError,
	isLoading,
	onSaveModelOverride,
	isSaving,
	isSaveError,
	saveErrorMessage,
	unsetPlaceholder = "Use chat default",
	unavailableModelWarning = "The saved model is no longer enabled and will be ignored until you choose a new override.",
	disabled = false,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const hasLoadedModelOverride = modelOverrideData !== undefined;
	const isMalformedOverride = modelOverrideData?.is_malformed ?? false;
	const enabledModelOptions = enabledModelConfigs.map((modelConfig) => {
		const providerInfo = providerInfoByID.get(modelConfig.ai_provider_id);
		return {
			id: modelConfig.id,
			provider: providerInfo?.provider ?? "",
			providerId: modelConfig.ai_provider_id,
			providerLabel: providerInfo?.displayName,
			providerIcon: providerInfo?.icon,
			model: modelConfig.model,
			displayName: modelConfig.display_name.trim() || modelConfig.model,
			contextLimit: modelConfig.context_limit,
		};
	});

	const form = useFormik({
		enableReinitialize: true,
		initialValues: {
			model_config_id: modelOverrideData?.model_config_id ?? "",
		},
		onSubmit: (values, { resetForm }) => {
			onSaveModelOverride(
				{
					model_config_id: values.model_config_id,
				},
				{
					onSuccess: () => {
						showSavedState();
						resetForm({ values });
					},
				},
			);
		},
	});
	const isFormDisabled =
		disabled || isSaving || isLoading || !hasLoadedModelOverride;
	const canSave =
		hasLoadedModelOverride && !disabled && (form.dirty || isMalformedOverride);

	const isUnavailableSavedModel =
		form.values.model_config_id !== "" &&
		!enabledModelOptions.some(
			(option) => option.id === form.values.model_config_id,
		);

	return (
		<AgentSettingLayout
			title={title}
			description={description}
			showSave={canSave}
			isSaving={isSaving}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				isSaveError ? <p className="m-0">{saveErrorMessage}</p> : undefined
			}
		>
			<div className="flex w-[22rem] max-w-full flex-col gap-2">
				<ModelSelector
					options={enabledModelOptions}
					value={form.values.model_config_id}
					onValueChange={(value) =>
						void form.setFieldValue("model_config_id", value)
					}
					disabled={isFormDisabled}
					placeholder={
						isUnavailableSavedModel ? "Unavailable model" : unsetPlaceholder
					}
					emptyMessage={
						isLoading ? "Loading models..." : "No enabled models found."
					}
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm"
					contentClassName="min-w-[18rem]"
				/>
				<ModelOverrideAlerts
					isUnavailableSavedModel={isUnavailableSavedModel}
					unavailableMessage={unavailableModelWarning}
					isMalformedOverride={isMalformedOverride}
					malformedMessage="The saved override is malformed and is being treated as unset. Click Save to clear it."
					modelConfigsError={modelConfigsError}
				/>
			</div>
			<Button
				size="lg"
				variant="outline"
				type="button"
				onClick={() => {
					void form.setFieldValue("model_config_id", "");
				}}
				disabled={isFormDisabled}
				className="h-10"
			>
				Clear
			</Button>
		</AgentSettingLayout>
	);
};
