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
	models: readonly TypesGen.ChatModel[];
	modelsError: unknown;
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
	if (!overrideData) {
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

const getModelLabel = (model: TypesGen.ChatModel): string => {
	return model.display_name.trim() || model.model || model.id;
};

const getModelLabelByID = (
	modelID: string,
	models: readonly TypesGen.ChatModel[],
): string | undefined => {
	const model = models.find((model) => model.id === modelID);
	return model ? getModelLabel(model) : undefined;
};

const getUnavailableModelLabel = (
	modelID: string,
	models: readonly TypesGen.ChatModel[],
): string => {
	const modelLabel = getModelLabelByID(modelID, models);
	if (!modelLabel) {
		return `Unavailable model (${modelID})`;
	}
	return `Unavailable: ${modelLabel}`;
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
	models: readonly TypesGen.ChatModel[],
): string => {
	if (context !== "root") {
		return "Your current chat model";
	}
	const defaultModel = models.find((model) => model.is_default);
	return defaultModel
		? getModelLabel(defaultModel)
		: "Model definition default";
};

const getDeploymentDefaultDescription = (
	deploymentDefault: TypesGen.ChatModelOverrideResponse | undefined,
	models: readonly TypesGen.ChatModel[],
): string => {
	if (!deploymentDefault) {
		return "Loading organization default";
	}
	const modelID = deploymentDefault.model_config_id.trim();
	if (modelID === "") {
		return "Chat default fallback";
	}
	return getModelLabelByID(modelID, models) ?? `Unavailable model (${modelID})`;
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
	models,
	modelsError,
	isLoading,
	onSave,
	isSaving,
	isSaveError,
	saveErrorMessage,
	disabled,
}) => {
	const hasLoadedOverride = overrideData !== undefined;
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
	const canSave = hasLoadedOverride && !disabled && form.dirty;
	const defaultModeOptions = getDefaultModeOptions(context).map((mode) => {
		const label =
			mode === "deployment_default" ? "Organization default" : "Chat default";
		const modeDescription =
			mode === "deployment_default"
				? getDeploymentDefaultDescription(deploymentDefault, models)
				: getChatDefaultDescription(context, models);
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
							? "Invalid organization default"
							: isUnavailableSelectedModel
								? getUnavailableModelLabel(form.values.model_config_id, models)
								: "Select..."
					}
					triggerAriaLabel={`${title} behavior`}
					emptyMessage="No matching models found."
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-xs md:w-[18rem]"
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
					modelsError={modelsError}
				>
					{isInvalidRootDeploymentDefault && (
						<Alert severity="warning">
							<AlertDescription>
								The saved root override uses the organization default, which is
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
