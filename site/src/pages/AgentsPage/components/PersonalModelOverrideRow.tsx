import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { pickReasoningEffort } from "../utils/reasoningEffort";
import { ModelSelector, type ModelSelectorOption } from "./ChatElements";
import { ModelOverrideAlerts } from "./ModelOverrideAlerts";
import { SectionHeader } from "./SectionHeader";

type PersonalOverrideContext = TypesGen.ChatPersonalModelOverrideContext;
type PersonalOverrideMode = TypesGen.ChatPersonalModelOverrideMode;
type PersonalOverride = TypesGen.ChatPersonalModelOverride;
type UpdatePersonalOverrideRequest =
	TypesGen.UpdateUserChatPersonalModelOverrideRequest;

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export type SavePersonalOverride = (
	req: UpdatePersonalOverrideRequest,
	options?: MutationCallbacks,
) => void;

interface PersonalOverrideFormValues {
	mode: PersonalOverrideMode;
	model_config_id: string;
	reasoning_effort: string;
}

interface PersonalModelOverrideRowProps {
	context: PersonalOverrideContext;
	title: string;
	description: string;
	overrideData: PersonalOverride | undefined;
	deploymentDefault?: TypesGen.ChatModelOverrideResponse;
	modelOptions: readonly ModelSelectorOption[];
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError: unknown;
	isLoading: boolean;
	onSave: SavePersonalOverride;
	isSaving: boolean;
	isSaveError: boolean;
	saveErrorMessage: string;
	disabled: boolean;
}

const getDefaultMode = (
	context: PersonalOverrideContext,
): PersonalOverrideMode => {
	return context === "root" ? "chat_default" : "deployment_default";
};

const toFormValues = (
	overrideData: PersonalOverride | undefined,
	context: PersonalOverrideContext,
): PersonalOverrideFormValues => {
	if (!overrideData || overrideData.is_malformed) {
		return {
			mode: getDefaultMode(context),
			model_config_id: "",
			reasoning_effort: "",
		};
	}
	return {
		mode: overrideData.mode,
		model_config_id:
			overrideData.mode === "model" ? overrideData.model_config_id : "",
		reasoning_effort:
			overrideData.mode === "model"
				? (overrideData.reasoning_effort ?? "")
				: "",
	};
};

const toUpdateRequest = (
	values: PersonalOverrideFormValues,
): UpdatePersonalOverrideRequest => {
	if (values.mode === "model") {
		return {
			mode: "model",
			model_config_id: values.model_config_id,
			...(values.reasoning_effort
				? { reasoning_effort: values.reasoning_effort }
				: {}),
		};
	}
	return { mode: values.mode, model_config_id: "" };
};

const getModelConfigLabel = (modelConfig: TypesGen.ChatModelConfig): string => {
	return modelConfig.display_name.trim() || modelConfig.model || modelConfig.id;
};

const getModelConfigLabelByID = (
	modelConfigID: string,
	modelConfigs: readonly TypesGen.ChatModelConfig[],
): string | undefined => {
	const modelConfig = modelConfigs.find(
		(config) => config.id === modelConfigID,
	);
	return modelConfig ? getModelConfigLabel(modelConfig) : undefined;
};

const getUnavailableModelLabel = (
	modelConfigID: string,
	modelConfigs: readonly TypesGen.ChatModelConfig[],
): string => {
	const modelConfigLabel = getModelConfigLabelByID(modelConfigID, modelConfigs);
	if (!modelConfigLabel) {
		return `Unavailable model (${modelConfigID})`;
	}
	return `Unavailable: ${modelConfigLabel}`;
};

const getDefaultModeOptions = (
	context: PersonalOverrideContext,
): readonly Exclude<PersonalOverrideMode, "model">[] => {
	return context === "root"
		? ["chat_default"]
		: ["deployment_default", "chat_default"];
};

const getChatDefaultDescription = (
	context: PersonalOverrideContext,
	modelConfigs: readonly TypesGen.ChatModelConfig[],
): string => {
	if (context !== "root") {
		return "Your current chat model";
	}
	const defaultModel = modelConfigs.find((config) => config.is_default);
	return defaultModel
		? getModelConfigLabel(defaultModel)
		: "Model definition default";
};

const getDeploymentDefaultDescription = (
	deploymentDefault: TypesGen.ChatModelOverrideResponse | undefined,
	modelConfigs: readonly TypesGen.ChatModelConfig[],
): string => {
	if (!deploymentDefault) {
		return "Loading deployment default";
	}
	if (deploymentDefault.is_malformed) {
		return "Invalid deployment default";
	}
	const modelConfigID = deploymentDefault.model_config_id.trim();
	if (modelConfigID === "") {
		return "Chat default fallback";
	}
	return (
		getModelConfigLabelByID(modelConfigID, modelConfigs) ??
		`Unavailable model (${modelConfigID})`
	);
};

const isDefaultModeOption = (
	value: string,
): value is Exclude<PersonalOverrideMode, "model"> => {
	return value === "chat_default" || value === "deployment_default";
};

export const PersonalModelOverrideRow: FC<PersonalModelOverrideRowProps> = ({
	context,
	title,
	description,
	overrideData,
	deploymentDefault,
	modelOptions,
	modelConfigs,
	modelConfigsError,
	isLoading,
	onSave,
	isSaving,
	isSaveError,
	saveErrorMessage,
	disabled,
}) => {
	const hasLoadedOverride = overrideData !== undefined;
	const isMalformedOverride = overrideData?.is_malformed ?? false;
	const form = useFormik<PersonalOverrideFormValues>({
		enableReinitialize: true,
		initialValues: toFormValues(overrideData, context),
		onSubmit: (values, { resetForm }) => {
			onSave(toUpdateRequest(values), {
				onSuccess: () => resetForm({ values }),
			});
		},
	});
	const isFormDisabled =
		disabled || isSaving || isLoading || !hasLoadedOverride;
	const canSave =
		hasLoadedOverride && !disabled && (form.dirty || isMalformedOverride);
	const defaultModeOptions = getDefaultModeOptions(context).map((mode) => {
		const label =
			mode === "deployment_default" ? "Deployment default" : "Chat default";
		const modeDescription =
			mode === "deployment_default"
				? getDeploymentDefaultDescription(deploymentDefault, modelConfigs)
				: getChatDefaultDescription(context, modelConfigs);
		return {
			id: mode,
			provider: "defaults",
			providerLabel: "Defaults",
			model: mode,
			displayName: `${label}: ${modeDescription}`,
		};
	});
	const isInvalidRootDeploymentDefault =
		context === "root" && overrideData?.mode === "deployment_default";
	const isUnavailableSavedModel =
		overrideData?.mode === "model" &&
		overrideData.is_set &&
		overrideData.model_config_id.trim() !== "" &&
		!modelOptions.some((option) => option.id === overrideData.model_config_id);
	const isUnavailableSelectedModel =
		form.values.mode === "model" &&
		form.values.model_config_id.trim() !== "" &&
		!modelOptions.some((option) => option.id === form.values.model_config_id);
	const selectionValue =
		form.values.mode === "model"
			? form.values.model_config_id
			: form.values.mode;
	const selectedModelOption = modelOptions.find(
		(option) => option.id === form.values.model_config_id,
	);
	const selectedReasoningEffort =
		form.values.mode === "model" && selectedModelOption
			? pickReasoningEffort(
					form.values.reasoning_effort,
					selectedModelOption.reasoningEfforts ?? [],
					selectedModelOption.reasoningEffortDefault,
				)
			: undefined;
	const canSaveSelection =
		canSave &&
		(form.values.mode !== "model" ||
			(form.values.model_config_id.trim() !== "" &&
				!isUnavailableSelectedModel));

	return (
		<section aria-label={title} className="flex flex-col gap-3">
			<SectionHeader label={title} description={description} level="section" />
			<form className="flex flex-col gap-3" onSubmit={form.handleSubmit}>
				<ModelSelector
					options={[...defaultModeOptions, ...modelOptions]}
					value={selectionValue}
					onValueChange={(value) => {
						if (isDefaultModeOption(value)) {
							void form.setValues({
								mode: value,
								model_config_id: "",
								reasoning_effort: "",
							});
							return;
						}
						const option = modelOptions.find((option) => option.id === value);
						let reasoningEffort = "";
						if (option) {
							reasoningEffort =
								pickReasoningEffort(
									"",
									option.reasoningEfforts ?? [],
									option.reasoningEffortDefault,
								) ?? "";
						}
						void form.setValues({
							mode: "model",
							model_config_id: value,
							reasoning_effort: reasoningEffort,
						});
					}}
					disabled={isFormDisabled}
					placeholder={
						isInvalidRootDeploymentDefault
							? "Invalid deployment default"
							: isUnavailableSelectedModel
								? getUnavailableModelLabel(
										form.values.model_config_id,
										modelConfigs,
									)
								: "Select..."
					}
					triggerAriaLabel={`${title} behavior`}
					emptyMessage="No matching models found."
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-sm md:w-[18rem]"
					contentClassName="min-w-[18rem]"
					reasoningEffort={selectedReasoningEffort}
					onReasoningEffortChange={(value) =>
						void form.setFieldValue("reasoning_effort", value)
					}
				/>
				{modelOptions.length === 0 && (
					<p role="status" className="m-0 text-xs text-content-secondary">
						{isLoading ? "Loading models..." : "No enabled models found."}
					</p>
				)}

				<ModelOverrideAlerts
					isUnavailableSavedModel={isUnavailableSavedModel}
					unavailableMessage="The saved model is unavailable and will be ignored until you choose a valid model override."
					isMalformedOverride={isMalformedOverride}
					malformedMessage="The saved override is malformed. Choose a valid value and save to replace it."
					modelConfigsError={modelConfigsError}
				>
					{isInvalidRootDeploymentDefault && (
						<Alert severity="warning">
							<AlertDescription>
								The saved root override uses the deployment default, which is
								not supported for root agents. Choose a valid value and save to
								replace it.
							</AlertDescription>
						</Alert>
					)}
				</ModelOverrideAlerts>
				<div className="flex justify-end">
					<Button
						size="sm"
						type="submit"
						disabled={isFormDisabled || !canSaveSelection}
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
		</section>
	);
};
