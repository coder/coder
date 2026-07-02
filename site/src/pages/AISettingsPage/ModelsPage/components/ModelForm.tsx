import { useFormik } from "formik";
import { type FC, useRef, useState } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import {
	canManageProviderModels,
	type ProviderState,
} from "#/modules/aiModels/providerStates";
import {
	buildInitialModelFormValues,
	buildModelConfigFromForm,
	type ModelFormValues,
	parsePositiveInteger,
	parseThresholdInteger,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/modelConfigFormLogic";
import { getFormHelpers } from "#/utils/formUtils";
import { ModelFormDialogs } from "./ModelFormDialogs";
import { ModelFormFields } from "./ModelFormFields";
import { ModelFormBackLink, ModelFormHeader } from "./ModelFormHeader";
import { ModelFormProviderSelect } from "./ModelFormProviderSelect";

const indefiniteArticle = (word: string): string =>
	/^[aeiou]/i.test(word) ? "an" : "a";

const validationSchema = Yup.object({
	model: Yup.string().trim().required("Model ID is required."),
	displayName: Yup.string(),
	enabled: Yup.boolean(),
	contextLimit: Yup.string()
		.required("Context limit is required.")
		.test(
			"positive-integer",
			"Context limit must be a positive integer.",
			(value) => !value?.trim() || parsePositiveInteger(value) !== null,
		),
	compressionThreshold: Yup.string().test(
		"threshold-range",
		"Compression threshold must be a number between 0 and 100.",
		(value) => !value?.trim() || parseThresholdInteger(value) !== null,
	),
	isDefault: Yup.boolean(),
});

interface ModelFormProps {
	editingModel?: TypesGen.ChatModelConfig;
	duplicateSourceModel?: TypesGen.ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState | null;
	onProviderChange: (providerKey: string) => void;
	isSaving: boolean;
	isDeleting: boolean;
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onDeleteModel?: (modelConfigId: string) => Promise<void>;
	currentDefaultModel?: TypesGen.ChatModelConfig;
	onSetDefault?: () => void;
	onDuplicate?: () => void;
	onToggleEnabled?: (enabled: boolean) => void;
}

export const ModelForm: FC<ModelFormProps> = ({
	editingModel,
	duplicateSourceModel,
	providerStates,
	selectedProviderState,
	onProviderChange,
	isSaving,
	isDeleting,
	onCreateModel,
	onUpdateModel,
	onDeleteModel,
	onDuplicate,
	currentDefaultModel,
	onToggleEnabled,
}) => {
	const initialModel = editingModel ?? duplicateSourceModel;
	const isEditing = Boolean(editingModel);
	const isDuplicating = Boolean(duplicateSourceModel) && !isEditing;
	const initialValues = {
		...buildInitialModelFormValues(initialModel),
		...(isDuplicating && { isDefault: false }),
	};
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [showPricing, setShowPricing] = useState(false);
	const [showProviderConfig, setShowProviderConfig] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);
	const [confirmingReplaceDefault, setConfirmingReplaceDefault] =
		useState(false);
	const replaceDefaultConfirmedRef = useRef(false);

	const canAddModelForSelectedProvider = canManageProviderModels(
		selectedProviderState ?? undefined,
	);
	const mode: "add" | "edit" | "duplicate" = isEditing
		? "edit"
		: isDuplicating
			? "duplicate"
			: "add";

	const selectedProviderType =
		selectedProviderState?.provider ?? selectedProviderState?.key ?? "";
	const selectedProviderKey = selectedProviderState?.key ?? "";

	const form = useFormik<ModelFormValues>({
		initialValues,
		validationSchema,
		validateOnMount: true,
		validateOnBlur: false,
		onSubmit: async (values) => {
			if (isSaving) return;

			const replacingDefault =
				values.isDefault &&
				currentDefaultModel != null &&
				currentDefaultModel.id !== editingModel?.id;
			if (replacingDefault && !replaceDefaultConfirmedRef.current) {
				setConfirmingReplaceDefault(true);
				return;
			}
			replaceDefaultConfirmedRef.current = false;

			const trimmedModel = values.model.trim();
			if (!trimmedModel) return;

			const parsedContextLimit = parsePositiveInteger(values.contextLimit);
			const parsedCompressionThreshold = parseThresholdInteger(
				values.compressionThreshold,
			);

			const buildResult = buildModelConfigFromForm(
				selectedProviderType,
				values.config,
			);
			if (Object.keys(buildResult.fieldErrors).length > 0) return;

			const trimmedDisplayName = values.displayName.trim();
			const builtModelConfig = buildResult.modelConfig;

			const selectedProviderConfigID =
				selectedProviderState?.providerConfig?.id;
			const editingProviderConfigID = editingModel?.ai_provider_id.trim() ?? "";

			if (isEditing && editingModel) {
				const req: TypesGen.UpdateChatModelConfigRequest = {
					...(selectedProviderConfigID &&
						selectedProviderConfigID !== editingProviderConfigID && {
							ai_provider_id: selectedProviderConfigID,
						}),
					...(trimmedModel !== editingModel.model && {
						model: trimmedModel,
					}),
					...(trimmedDisplayName !== (editingModel.display_name ?? "") && {
						display_name: trimmedDisplayName,
					}),
					...(parsedContextLimit !== null &&
						parsedContextLimit !== editingModel.context_limit && {
							context_limit: parsedContextLimit,
						}),
					...(parsedCompressionThreshold !== null &&
						parsedCompressionThreshold !==
							editingModel.compression_threshold && {
							compression_threshold: parsedCompressionThreshold,
						}),
					...(values.isDefault !== editingModel.is_default && {
						is_default: values.isDefault,
					}),
					model_config: builtModelConfig,
				};

				await onUpdateModel(editingModel.id, req);
			} else {
				if (!selectedProviderState?.providerConfig) return;

				const req: TypesGen.CreateChatModelConfigRequest = {
					ai_provider_id: selectedProviderState.providerConfig.id,
					model: trimmedModel,
					enabled: values.enabled,
					is_default: values.isDefault,
					...(parsedContextLimit !== null && {
						context_limit: parsedContextLimit,
					}),
					...(parsedCompressionThreshold !== null && {
						compression_threshold: parsedCompressionThreshold,
					}),
					...(trimmedDisplayName && {
						display_name: trimmedDisplayName,
					}),
					...(builtModelConfig && {
						model_config: builtModelConfig,
					}),
				};

				await onCreateModel(req);
			}
		},
	});

	const getFieldHelpers = getFormHelpers(form);

	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	const modelConfigFormBuildResult = buildModelConfigFromForm(
		selectedProviderType,
		form.values.config,
	);
	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;
	const enabledToggleDisabled =
		isSaving ||
		(editingModel?.is_default === true && editingModel.enabled === true);
	const setDefaultDisabled =
		isSaving ||
		(isEditing &&
			(editingModel?.is_default === true || editingModel?.enabled === false));

	const contextLimitValid =
		parsePositiveInteger(form.values.contextLimit) !== null;
	const compressionThresholdValid =
		!form.values.compressionThreshold.trim() ||
		parseThresholdInteger(form.values.compressionThreshold) !== null;
	const canSubmit =
		!isSaving &&
		!hasFieldErrors &&
		form.values.model.trim().length > 0 &&
		contextLimitValid &&
		compressionThresholdValid &&
		(!isEditing || form.dirty);

	const handleConfirmReplaceDefault = () => {
		replaceDefaultConfirmedRef.current = true;
		setConfirmingReplaceDefault(false);
		void form.submitForm();
	};

	if (
		!selectedProviderState ||
		(!canAddModelForSelectedProvider && !isEditing)
	) {
		return (
			<>
				<ModelFormBackLink />
				<div className="flex flex-col gap-6 pt-6">
					<SettingsHeaderTitle>Add model</SettingsHeaderTitle>
					<div className="border border-solid p-6 rounded-lg">
						<div className="space-y-3">
							<ModelFormProviderSelect
								providerStates={providerStates}
								selectedProviderKey={selectedProviderKey}
								onProviderChange={onProviderChange}
								disabled={isDuplicating || providerStates.length === 0}
							/>
							{selectedProviderState && (
								<p className="text-sm text-content-secondary m-0">
									{!selectedProviderState.providerConfig
										? "Create a managed provider before adding models."
										: "Set an API key for this provider before adding models."}
								</p>
							)}
						</div>
					</div>
				</div>
			</>
		);
	}

	const modelField = getFieldHelpers("model");
	const contextLimitField = getFieldHelpers("contextLimit");
	const compressionThresholdField = getFieldHelpers("compressionThreshold");
	const displayNameField = getFieldHelpers("displayName");

	const providerLabel = selectedProviderState.label;
	const title = isEditing
		? editingModel
			? editingModel.display_name || editingModel.model
			: "Edit model"
		: isDuplicating
			? `Duplicate ${providerLabel} model`
			: `Add ${indefiniteArticle(providerLabel)} ${providerLabel} model`;

	return (
		<>
			<ModelFormHeader
				title={title}
				selectedProviderState={selectedProviderState}
				isEditing={isEditing}
				editingModel={editingModel}
				onDeleteModel={onDeleteModel}
				onDuplicate={onDuplicate}
				onToggleEnabled={onToggleEnabled}
				isSaving={isSaving}
				enabledToggleDisabled={enabledToggleDisabled}
				onRequestDelete={() => setConfirmingDelete(true)}
			/>
			<div className="flex flex-col gap-6 pt-6">
				<ModelFormFields
					form={form}
					mode={mode}
					providerStates={providerStates}
					selectedProviderState={selectedProviderState}
					selectedProviderKey={selectedProviderKey}
					selectedProviderType={selectedProviderType}
					onProviderChange={onProviderChange}
					isDuplicating={isDuplicating}
					isEditing={isEditing}
					isSaving={isSaving}
					canSubmit={canSubmit}
					initialModel={initialModel}
					modelField={modelField}
					contextLimitField={contextLimitField}
					compressionThresholdField={compressionThresholdField}
					displayNameField={displayNameField}
					setDefaultDisabled={setDefaultDisabled}
					modelConfigFormBuildResult={modelConfigFormBuildResult}
					showPricing={showPricing}
					setShowPricing={setShowPricing}
					showProviderConfig={showProviderConfig}
					setShowProviderConfig={setShowProviderConfig}
					showAdvanced={showAdvanced}
					setShowAdvanced={setShowAdvanced}
				/>
			</div>
			<ModelFormDialogs
				editingModel={editingModel}
				onDeleteModel={onDeleteModel}
				isDeleting={isDeleting}
				confirmingDelete={confirmingDelete}
				setConfirmingDelete={setConfirmingDelete}
				resetForm={(values) => form.resetForm({ values })}
				formValues={form.values}
				unsavedChanges={unsavedChanges}
				confirmingReplaceDefault={confirmingReplaceDefault}
				setConfirmingReplaceDefault={setConfirmingReplaceDefault}
				currentDefaultModel={currentDefaultModel}
				onConfirmReplaceDefault={handleConfirmReplaceDefault}
			/>
		</>
	);
};
