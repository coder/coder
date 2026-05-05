import { useFormik } from "formik";
import { Select as SelectPrimitive } from "radix-ui";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import type { ModelSelectorOption } from "./ChatElements";
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
		return { mode: getDefaultMode(context), model_config_id: "" };
	}
	return {
		mode: overrideData.mode,
		model_config_id:
			overrideData.mode === "model" ? overrideData.model_config_id : "",
	};
};

const toUpdateRequest = (
	values: PersonalOverrideFormValues,
): UpdatePersonalOverrideRequest => {
	if (values.mode === "model") {
		return {
			mode: "model",
			model_config_id: values.model_config_id,
		};
	}
	return { mode: values.mode, model_config_id: "" };
};

const getModelConfigLabel = (modelConfig: TypesGen.ChatModelConfig): string => {
	return modelConfig.display_name.trim() || modelConfig.model || modelConfig.id;
};

const getModelOptionLabel = (option: ModelSelectorOption): string => {
	return option.displayName.trim() || option.model || option.id;
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

const getSelectionLabel = ({
	context,
	deploymentDefault,
	isInvalidRootDeploymentDefault,
	modelConfigs,
	modelOptions,
	values,
}: {
	context: PersonalOverrideContext;
	deploymentDefault?: TypesGen.ChatModelOverrideResponse;
	isInvalidRootDeploymentDefault: boolean;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelOptions: readonly ModelSelectorOption[];
	values: PersonalOverrideFormValues;
}): string => {
	if (isInvalidRootDeploymentDefault) {
		return "Invalid deployment default";
	}

	switch (values.mode) {
		case "chat_default":
			return `Chat default: ${getChatDefaultDescription(context, modelConfigs)}`;
		case "deployment_default":
			return `Deployment default: ${getDeploymentDefaultDescription(
				deploymentDefault,
				modelConfigs,
			)}`;
		case "model": {
			const modelConfigID = values.model_config_id.trim();
			const modelOption = modelOptions.find(
				(option) => option.id === modelConfigID,
			);
			if (modelOption) {
				return getModelOptionLabel(modelOption);
			}
			return modelConfigID === ""
				? "Select..."
				: getUnavailableModelLabel(modelConfigID, modelConfigs);
		}
	}
};

const isDefaultModeOption = (
	value: string,
): value is Exclude<PersonalOverrideMode, "model"> => {
	return value === "chat_default" || value === "deployment_default";
};

// Local separator for use inside SelectContent. Defined here instead of
// in the core Select component so the styling stays scoped to this
// feature until a shared design lands.
const SelectSeparator: FC = () => (
	<SelectPrimitive.Separator className="-mx-1 my-1 h-px bg-border" />
);

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
	const defaultModeOptions = getDefaultModeOptions(context);
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
	const selectionLabel = getSelectionLabel({
		context,
		deploymentDefault,
		isInvalidRootDeploymentDefault,
		modelConfigs,
		modelOptions,
		values: form.values,
	});
	const canSaveSelection =
		canSave &&
		(form.values.mode !== "model" ||
			(form.values.model_config_id.trim() !== "" &&
				!isUnavailableSelectedModel));

	return (
		<section aria-label={title} className="flex flex-col gap-3">
			<SectionHeader label={title} description={description} level="section" />
			<form className="flex flex-col gap-3" onSubmit={form.handleSubmit}>
				<Select
					value={selectionValue}
					onValueChange={(value) => {
						if (isDefaultModeOption(value)) {
							void form.setValues({ mode: value, model_config_id: "" });
							return;
						}
						void form.setValues({ mode: "model", model_config_id: value });
					}}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label={`${title} behavior`}
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-sm md:w-[18rem]"
					>
						<SelectValue placeholder="Select...">{selectionLabel}</SelectValue>
					</SelectTrigger>
					<SelectContent className="min-w-[18rem]">
						{isInvalidRootDeploymentDefault && (
							<>
								<SelectItem value="deployment_default" disabled>
									Invalid deployment default
								</SelectItem>
								<SelectSeparator />
							</>
						)}
						<SelectGroup>
							{defaultModeOptions.map((mode) => (
								<DefaultModeSelectItem
									key={mode}
									mode={mode}
									context={context}
									deploymentDefault={deploymentDefault}
									modelConfigs={modelConfigs}
								/>
							))}
						</SelectGroup>
						<SelectSeparator />
						{isUnavailableSelectedModel && (
							<>
								<SelectItem value={form.values.model_config_id} disabled>
									{getUnavailableModelLabel(
										form.values.model_config_id,
										modelConfigs,
									)}
								</SelectItem>
								<SelectSeparator />
							</>
						)}
						<SelectGroup>
							{modelOptions.map((option) => (
								<SelectItem key={option.id} value={option.id}>
									{getModelOptionLabel(option)}
								</SelectItem>
							))}
							{modelOptions.length === 0 && (
								<SelectItem value="__empty_models__" disabled>
									{isLoading ? "Loading models..." : "No enabled models found."}
								</SelectItem>
							)}
						</SelectGroup>
					</SelectContent>
				</Select>
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

interface DefaultModeSelectItemProps {
	mode: Exclude<PersonalOverrideMode, "model">;
	context: PersonalOverrideContext;
	deploymentDefault?: TypesGen.ChatModelOverrideResponse;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
}

const DefaultModeSelectItem: FC<DefaultModeSelectItemProps> = ({
	mode,
	context,
	deploymentDefault,
	modelConfigs,
}) => {
	const label =
		mode === "deployment_default" ? "Deployment default" : "Chat default";
	const description =
		mode === "deployment_default"
			? getDeploymentDefaultDescription(deploymentDefault, modelConfigs)
			: getChatDefaultDescription(context, modelConfigs);

	return (
		<SelectItem value={mode}>
			<span className="flex min-w-0 flex-col">
				<span className="truncate text-content-primary">{label}</span>
				<span className="truncate text-content-secondary text-xs leading-tight">
					{description}
				</span>
			</span>
		</SelectItem>
	);
};
