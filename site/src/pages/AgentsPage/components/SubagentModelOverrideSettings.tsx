import { useFormik } from "formik";
import type { FC, ReactNode } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import type { ModelSelectorOption } from "./ChatElements/ModelSelector";
import { ModelSelector } from "./ChatElements/ModelSelector";

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
	description: ReactNode;
	modelOverrideData: ModelOverrideData | undefined;
	enabledModelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError: unknown;
	isLoading: boolean;
	onSaveModelOverride: (
		req: UpdateModelOverrideRequest,
		options?: MutationCallbacks,
	) => void;
	isSaving: boolean;
	isSaveError: boolean;
	saveErrorMessage: string;
	showHeader?: boolean;
	disabled?: boolean;
}

const toModelSelectorOption = (
	modelConfig: TypesGen.ChatModelConfig,
): ModelSelectorOption => ({
	id: modelConfig.id,
	provider: modelConfig.provider,
	model: modelConfig.model,
	displayName: modelConfig.display_name.trim() || modelConfig.model,
	contextLimit: modelConfig.context_limit,
});

export const SubagentModelOverrideSettings: FC<
	SubagentModelOverrideSettingsProps
> = ({
	title,
	description,
	modelOverrideData,
	enabledModelConfigs,
	modelConfigsError,
	isLoading,
	onSaveModelOverride,
	isSaving,
	isSaveError,
	saveErrorMessage,
	showHeader = true,
	disabled = false,
}) => {
	const hasLoadedModelOverride = modelOverrideData !== undefined;
	const enabledModelOptions = enabledModelConfigs.map(toModelSelectorOption);

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
						resetForm({ values });
					},
				},
			);
		},
	});

	const isUnavailableSavedModel =
		form.values.model_config_id !== "" &&
		!enabledModelOptions.some(
			(option) => option.id === form.values.model_config_id,
		);
	const isMalformedOverride = modelOverrideData?.is_malformed ?? false;
	const isModelOverrideDisabled =
		disabled || isSaving || isLoading || !hasLoadedModelOverride;
	const canSaveModelOverride =
		hasLoadedModelOverride && (form.dirty || isMalformedOverride);

	return (
		<form aria-label={title} className="space-y-2" onSubmit={form.handleSubmit}>
			{showHeader && (
				<>
					<h3 className="m-0 text-[13px] font-semibold text-content-primary">
						{title}
					</h3>
					<p className="!mt-0.5 m-0 text-xs text-content-secondary">
						{description}
					</p>
				</>
			)}
			<ModelSelector
				options={enabledModelOptions}
				value={form.values.model_config_id}
				onValueChange={(value) => form.setFieldValue("model_config_id", value)}
				disabled={isModelOverrideDisabled}
				placeholder={
					isUnavailableSavedModel ? "Unavailable model" : "Use chat default"
				}
				emptyMessage={
					isLoading ? "Loading models..." : "No enabled models found."
				}
				className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-sm"
				contentClassName="min-w-[18rem]"
			/>
			{isUnavailableSavedModel && (
				<Alert severity="warning">
					<AlertDescription>
						The saved model is no longer enabled and will be ignored until you
						choose a new override.
					</AlertDescription>
				</Alert>
			)}
			{isMalformedOverride && (
				<Alert severity="warning">
					<AlertDescription>
						The saved override is malformed and is being treated as unset. Click
						Save to clear it.
					</AlertDescription>
				</Alert>
			)}
			{Boolean(modelConfigsError) && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load model configs.
				</p>
			)}
			<div className="flex justify-end gap-2">
				<Button
					size="sm"
					variant="outline"
					type="button"
					onClick={() => {
						form.setFieldValue("model_config_id", "");
					}}
					disabled={isModelOverrideDisabled}
				>
					Clear
				</Button>
				<Button
					size="sm"
					type="submit"
					disabled={isModelOverrideDisabled || !canSaveModelOverride}
				>
					Save
				</Button>
			</div>
			{isSaveError && (
				<p className="m-0 text-xs text-content-destructive">
					{saveErrorMessage}
				</p>
			)}
		</form>
	);
};
