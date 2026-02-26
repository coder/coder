import { getErrorMessage } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { useFormik } from "formik";
import { ArrowLeftIcon, Loader2Icon, PlusIcon, SaveIcon } from "lucide-react";
import { type FC, useMemo } from "react";
import { toast } from "sonner";
import { cn } from "utils/cn";
import { getFormHelpers } from "utils/formUtils";
import * as Yup from "yup";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelConfigFields } from "./ModelConfigFields";
import {
	buildInitialModelFormValues,
	buildModelConfigFromForm,
	type ModelFormValues,
	parsePositiveInteger,
	parseThresholdInteger,
} from "./modelConfigFormLogic";
import { getModelConfigSchemaReference } from "./modelConfigSchemas";
import { ProviderIcon } from "./ProviderIcon";

// ── Validation ──────────────────────────────────────────────────

const makeValidationSchema = (isEditing: boolean) =>
	Yup.object({
		model: Yup.string().trim().required("Model ID is required."),
		displayName: Yup.string(),
		contextLimit: isEditing
			? Yup.string()
					.trim()
					.required("Context limit is required.")
					.test(
						"positive-integer",
						"Context limit must be a positive integer.",
						(value) => !value || parsePositiveInteger(value) !== null,
					)
			: Yup.string().test(
					"positive-integer",
					"Context limit must be a positive integer.",
					(value) => !value?.trim() || parsePositiveInteger(value) !== null,
				),
		compressionThreshold: isEditing
			? Yup.string()
					.trim()
					.required("Compression threshold is required.")
					.test(
						"threshold-range",
						"Compression threshold must be a number between 0 and 100.",
						(value) => !value || parseThresholdInteger(value) !== null,
					)
			: Yup.string().test(
					"threshold-range",
					"Compression threshold must be a number between 0 and 100.",
					(value) => !value?.trim() || parseThresholdInteger(value) !== null,
				),
		isDefault: Yup.boolean(),
	});

// ── Component ──────────────────────────────────────────────────

type ModelFormProps = {
	/** When set, the form is in "edit" mode for the given model. */
	editingModel?: TypesGen.ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProvider: string | null;
	selectedProviderState: ProviderState | null;
	onSelectedProviderChange: (provider: string) => void;
	modelConfigsUnavailable: boolean;
	isSaving: boolean;
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
};

export const ModelForm: FC<ModelFormProps> = ({
	editingModel,
	providerStates,
	selectedProvider,
	selectedProviderState,
	onSelectedProviderChange,
	modelConfigsUnavailable,
	isSaving,
	onCreateModel,
	onUpdateModel,
	onCancel,
}) => {
	const isEditing = Boolean(editingModel);

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
	);

	const validationSchema = useMemo(
		() => makeValidationSchema(isEditing),
		[isEditing],
	);

	const form = useFormik<ModelFormValues>({
		initialValues: buildInitialModelFormValues(editingModel),
		validationSchema,
		validateOnMount: true,
		validateOnBlur: false,
		onSubmit: async (values) => {
			if (isSaving) return;

			const trimmedModel = values.model.trim();
			if (!trimmedModel) return;

			const parsedContextLimit = parsePositiveInteger(values.contextLimit);
			const parsedCompressionThreshold = parseThresholdInteger(
				values.compressionThreshold,
			);

			const buildResult = buildModelConfigFromForm(
				selectedProviderState?.provider,
				values.config,
			);
			if (Object.keys(buildResult.fieldErrors).length > 0) return;

			const trimmedDisplayName = values.displayName.trim();
			const builtModelConfig = buildResult.modelConfig;

			try {
				if (isEditing && editingModel) {
					const req: TypesGen.UpdateChatModelConfigRequest = {
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
						// Always send model_config so it can be cleared or updated.
						model_config: builtModelConfig,
					};

					await onUpdateModel(editingModel.id, req);
				} else {
					if (!selectedProviderState?.providerConfig) return;

					const req: TypesGen.CreateChatModelConfigRequest = {
						provider: selectedProviderState.provider,
						model: trimmedModel,
						...(parsedContextLimit !== null && {
							context_limit: parsedContextLimit,
						}),
						...(parsedCompressionThreshold !== null && {
							compression_threshold: parsedCompressionThreshold,
						}),
						...(trimmedDisplayName && {
							display_name: trimmedDisplayName,
						}),
						...(values.isDefault && {
							is_default: true,
						}),
						...(builtModelConfig && {
							model_config: builtModelConfig,
						}),
					};

					await onCreateModel(req);
				}
				// Navigation is handled by the parent (ModelsSection) after
				// the mutation promise resolves, so we do not call onCancel()
				// here to avoid a double view-transition.
			} catch (error) {
				toast.error(
					getErrorMessage(error, "Failed to save model configuration."),
				);
			}
		},
	});

	const getFieldHelpers = getFormHelpers(form);

	const modelConfigFormBuildResult = useMemo(
		() =>
			buildModelConfigFromForm(
				selectedProviderState?.provider,
				form.values.config,
			),
		[selectedProviderState?.provider, form.values.config],
	);

	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;

	const modelConfigSchemaReference = useMemo(
		() => getModelConfigSchemaReference(selectedProviderState),
		[selectedProviderState],
	);

	// ── Provider select (shared across all form states) ───────

	const providerSelect = (
		<div className="grid gap-1.5">
			<Label
				htmlFor="providerSelect"
				className="text-[13px] font-medium text-content-primary"
			>
				Provider
			</Label>
			<Select
				value={selectedProvider ?? ""}
				onValueChange={onSelectedProviderChange}
				disabled={isEditing || providerStates.length === 0}
			>
				<SelectTrigger
					id="providerSelect"
					className="h-10 max-w-[240px] text-[13px]"
				>
					<SelectValue placeholder="Select provider" />
				</SelectTrigger>
				<SelectContent>
					{providerStates.map((ps) => (
						<SelectItem key={ps.provider} value={ps.provider}>
							<span className="flex items-center gap-2">
								<ProviderIcon
									provider={ps.provider}
									className="h-4 w-4"
									active={ps.hasEffectiveAPIKey}
								/>
								{ps.label}
							</span>
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);

	// No provider selected or configs unavailable.
	if (!selectedProviderState || modelConfigsUnavailable) {
		return (
			<div className="flex h-full flex-col">
				<div className="flex items-center gap-2 border-b border-border px-6 py-4">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						{isEditing ? "Edit model" : "Add model"}
					</h3>
				</div>
				<div className="space-y-3 p-6">{providerSelect}</div>
			</div>
		);
	}

	// Provider can't manage models.
	if (!canManageModels && !isEditing) {
		return (
			<div className="flex h-full flex-col">
				<div className="flex items-center gap-2 border-b border-border px-6 py-4">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						Add model
					</h3>
				</div>
				<div className="space-y-3 p-6">
					{providerSelect}
					<p className="text-[13px] text-content-secondary">
						{!selectedProviderState.providerConfig
							? "Create a managed provider config on the Providers tab before adding models."
							: "Set an API key for this provider on the Providers tab before adding models."}
					</p>
				</div>
			</div>
		);
	}

	// ── Full form ─────────────────────────────────────────────

	const modelField = getFieldHelpers("model");
	const displayNameField = getFieldHelpers("displayName");
	const contextLimitField = getFieldHelpers("contextLimit");
	const compressionThresholdField = getFieldHelpers("compressionThreshold");

	return (
		<div className="flex h-full flex-col">
			{/* Header bar with back button */}
			<div className="flex items-center justify-between gap-3 border-b border-border px-6 py-4">
				<div className="flex items-center gap-2">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						{isEditing ? "Edit model" : "Add model"}
					</h3>
					{selectedProviderState && (
						<span className="inline-flex items-center gap-1.5 rounded-md border border-border bg-surface-secondary/40 px-2 py-0.5 text-xs text-content-secondary">
							<ProviderIcon
								provider={selectedProviderState.provider}
								className="h-3.5 w-3.5"
								active
							/>
							{selectedProviderState.label}
						</span>
					)}
				</div>
			</div>

			{/* Form body */}
			<form
				className="flex min-h-0 flex-1 flex-col"
				onSubmit={form.handleSubmit}
			>
				<div className="flex-1 space-y-5 overflow-y-auto p-6">
					{/* Model identity */}
					<div className="space-y-3">
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								Model identity
							</p>
							<p className="m-0 text-xs text-content-secondary">
								Select provider and model naming details.
							</p>
						</div>
						<div className="grid items-start gap-3 md:grid-cols-3">
							{providerSelect}
							<div className="grid gap-1.5">
								<Label
									htmlFor={modelField.id}
									className="text-[13px] font-medium text-content-primary"
								>
									Model ID{" "}
									<span className="text-xs text-content-destructive font-bold">
										*
									</span>
								</Label>
								<Input
									id={modelField.id}
									name={modelField.name}
									className={cn(
										"h-10 text-[13px] placeholder:text-content-disabled",
										modelField.error && "border-content-destructive",
									)}
									placeholder="gpt-5, claude-sonnet-4-5, etc."
									value={modelField.value}
									onChange={modelField.onChange}
									onBlur={modelField.onBlur}
									disabled={isSaving}
									aria-invalid={modelField.error}
									aria-describedby={
										modelField.error ? `${modelField.id}-error` : undefined
									}
								/>
								{modelField.error && (
									<p
										id={`${modelField.id}-error`}
										className="m-0 text-xs text-content-destructive"
									>
										{modelField.helperText}
									</p>
								)}
							</div>
							<div className="grid gap-1.5">
								<Label
									htmlFor={displayNameField.id}
									className="text-[13px] font-medium text-content-primary"
								>
									Display name
								</Label>
								<Input
									id={displayNameField.id}
									name={displayNameField.name}
									className="h-10 text-[13px] placeholder:text-content-disabled"
									placeholder="Friendly label"
									value={displayNameField.value}
									onChange={displayNameField.onChange}
									onBlur={displayNameField.onBlur}
									disabled={isSaving}
								/>
							</div>
						</div>
					</div>

					{/* Runtime limits */}
					<div className="space-y-3">
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								Runtime limits
							</p>
							<p className="m-0 text-xs text-content-secondary">
								{isEditing
									? "These values are required for existing models."
									: "Leave values blank to use backend defaults."}
							</p>
						</div>
						<div className="grid gap-3 md:grid-cols-2">
							<div className="grid gap-1.5">
								<Label
									htmlFor={contextLimitField.id}
									className="text-[13px] font-medium text-content-primary"
								>
									Context limit{" "}
									{isEditing && (
										<span className="text-xs text-content-destructive font-bold">
											*
										</span>
									)}
								</Label>
								<Input
									id={contextLimitField.id}
									name={contextLimitField.name}
									className={cn(
										"h-10 text-[13px] placeholder:text-content-disabled",
										contextLimitField.error && "border-content-destructive",
									)}
									placeholder="200000"
									value={contextLimitField.value}
									onChange={contextLimitField.onChange}
									onBlur={contextLimitField.onBlur}
									disabled={isSaving}
									aria-invalid={contextLimitField.error}
									aria-describedby={
										contextLimitField.error
											? `${contextLimitField.id}-error`
											: undefined
									}
								/>
								{contextLimitField.error && (
									<p
										id={`${contextLimitField.id}-error`}
										className="m-0 text-xs text-content-destructive"
									>
										{contextLimitField.helperText}
									</p>
								)}
							</div>
							<div className="grid gap-1.5">
								<Label
									htmlFor={compressionThresholdField.id}
									className="text-[13px] font-medium text-content-primary"
								>
									Compression threshold{" "}
									{isEditing && (
										<span className="text-xs text-content-destructive font-bold">
											*
										</span>
									)}
								</Label>
								<Input
									id={compressionThresholdField.id}
									name={compressionThresholdField.name}
									className={cn(
										"h-10 text-[13px] placeholder:text-content-disabled",
										compressionThresholdField.error &&
											"border-content-destructive",
									)}
									placeholder="70"
									value={compressionThresholdField.value}
									onChange={compressionThresholdField.onChange}
									onBlur={compressionThresholdField.onBlur}
									disabled={isSaving}
									aria-invalid={compressionThresholdField.error}
									aria-describedby={
										compressionThresholdField.error
											? `${compressionThresholdField.id}-error`
											: undefined
									}
								/>
								{compressionThresholdField.error && (
									<p
										id={`${compressionThresholdField.id}-error`}
										className="m-0 text-xs text-content-destructive"
									>
										{compressionThresholdField.helperText}
									</p>
								)}
							</div>
						</div>
					</div>

					<div className="space-y-3">
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								Default behavior
							</p>
							<p className="m-0 text-xs text-content-secondary">
								Only one model can be the default for new prompts.
							</p>
						</div>
						<label
							htmlFor="isDefault"
							className="flex items-start gap-2 text-[13px] text-content-primary"
						>
							<Checkbox
								id="isDefault"
								checked={form.values.isDefault}
								onCheckedChange={(checked) =>
									void form.setFieldValue("isDefault", checked === true)
								}
								disabled={isSaving}
							/>
							<span>Use this as the default model for new prompts.</span>
						</label>
					</div>

					{/* Model call config fields */}
					<ModelConfigFields
						provider={selectedProviderState.provider}
						form={form}
						fieldErrors={modelConfigFormBuildResult.fieldErrors}
						disabled={isSaving}
					/>

					{/* Schema reference */}
					<details className="group rounded-xl border border-border-default/80 bg-surface-secondary/20 shadow-sm">
						<summary className="cursor-pointer select-none px-4 py-3 text-[13px] font-medium text-content-secondary hover:text-content-primary">
							Model config schema reference (
							{modelConfigSchemaReference.providerLabel})
						</summary>
						<div className="space-y-2 border-t border-border/60 px-4 pb-4 pt-3">
							<p className="m-0 text-xs text-content-secondary">
								Reference JSON for <code>create/update chat model config</code>{" "}
								payloads.
							</p>
							{modelConfigSchemaReference.notes.map((note) => (
								<p key={note} className="m-0 text-xs text-content-secondary">
									{note}
								</p>
							))}
							<pre
								data-testid="chat-model-config-schema"
								className="max-h-60 overflow-auto rounded-md border border-border-default/80 bg-surface-primary/80 p-2 font-mono text-[11px] leading-relaxed text-content-secondary"
							>
								{modelConfigSchemaReference.schemaJSON}
							</pre>
						</div>
					</details>
				</div>

				{/* Sticky footer actions */}
				<div className="flex items-center justify-end gap-2 border-t border-border bg-surface-primary px-6 py-4">
					<Button size="sm" variant="outline" type="button" onClick={onCancel}>
						Cancel
					</Button>
					<Button
						size="sm"
						type="submit"
						disabled={isSaving || !form.isValid || hasFieldErrors}
					>
						{isSaving ? (
							<Loader2Icon className="h-4 w-4 animate-spin" />
						) : isEditing ? (
							<SaveIcon className="h-4 w-4" />
						) : (
							<PlusIcon className="h-4 w-4" />
						)}
						{isEditing ? "Save changes" : "Add model"}
					</Button>
				</div>
			</form>
		</div>
	);
};
