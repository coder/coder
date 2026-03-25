import type * as TypesGen from "api/typesGenerated";
import { useFormik } from "formik";
import {
	ChevronDownIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { getFormHelpers } from "utils/formUtils";
import * as Yup from "yup";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import type { ProviderState } from "./ChatModelAdminPanel";
import {
	GeneralModelConfigFields,
	ModelConfigFields,
	PricingModelConfigFields,
} from "./ModelConfigFields";
import {
	buildInitialModelFormValues,
	buildModelConfigFromForm,
	type ModelFormValues,
	parsePositiveInteger,
	parseThresholdInteger,
} from "./modelConfigFormLogic";
import { ProviderIcon } from "./ProviderIcon";

// ── Validation ──────────────────────────────────────────────────

const validationSchema = Yup.object({
	model: Yup.string().trim().required("Model ID is required."),
	displayName: Yup.string(),
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

// ── Component ──────────────────────────────────────────────────

interface ModelFormProps {
	/** When set, the form is in "edit" mode for the given model. */
	editingModel?: TypesGen.ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProvider: string | null;
	selectedProviderState: ProviderState | null;
	onSelectedProviderChange: (provider: string) => void;
	modelConfigsUnavailable: boolean;
	isSaving: boolean;
	isDeleting: boolean;
	onCreateModel: (
		req: TypesGen.CreateChatModelConfigRequest,
	) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: TypesGen.UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
	onDeleteModel?: (modelConfigId: string) => Promise<void>;
}

export const ModelForm: FC<ModelFormProps> = ({
	editingModel,
	providerStates,
	selectedProvider,
	selectedProviderState,
	onSelectedProviderChange,
	modelConfigsUnavailable,
	isSaving,
	isDeleting,
	onCreateModel,
	onUpdateModel,
	onCancel,
	onDeleteModel,
}) => {
	const isEditing = Boolean(editingModel);
	const [showPricing, setShowPricing] = useState(false);
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
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
		},
	});

	const getFieldHelpers = getFormHelpers(form);

	const modelConfigFormBuildResult = buildModelConfigFromForm(
		selectedProviderState?.provider,
		form.values.config,
	);

	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;

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
								<ProviderIcon provider={ps.provider} className="h-4 w-4" />
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
			<div>
				<button
					type="button"
					onClick={onCancel}
					className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
				>
					<ChevronLeftIcon className="h-4 w-4" />
					Back
				</button>
				<h2 className="m-0 text-lg font-medium text-content-primary">
					{isEditing ? "Edit Model" : "Add Model"}
				</h2>
				<hr className="my-4 border-0 border-t border-solid border-border" />
				<div className="space-y-3">{providerSelect}</div>
			</div>
		);
	}

	// Provider can't manage models.
	if (!canManageModels && !isEditing) {
		return (
			<div>
				<button
					type="button"
					onClick={onCancel}
					className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
				>
					<ChevronLeftIcon className="h-4 w-4" />
					Back
				</button>
				<h2 className="m-0 text-lg font-medium text-content-primary">
					Add Model
				</h2>
				<hr className="my-4 border-0 border-t border-solid border-border" />
				<div className="space-y-3">
					{providerSelect}
					<p className="text-sm text-content-secondary">
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
	const contextLimitField = getFieldHelpers("contextLimit");
	const compressionThresholdField = getFieldHelpers("compressionThreshold");

	return (
		<div className="flex min-h-full flex-col">
			{/* Back */}
			<button
				type="button"
				onClick={onCancel}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>

			{/* Header — editable display name */}
			<div className="flex items-center gap-3">
				{selectedProviderState && (
					<ProviderIcon
						provider={selectedProviderState.provider}
						className="h-8 w-8"
					/>
				)}
				<div className="min-w-0 flex-1">
					<input
						type="text"
						{...form.getFieldProps("displayName")}
						disabled={isSaving}
						className="m-0 w-full border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
						placeholder={
							isEditing ? (editingModel?.model ?? "Model name") : "Model name"
						}
					/>
				</div>
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />

			{/* Form body */}
			<form className="flex flex-1 flex-col" onSubmit={form.handleSubmit}>
				<div className="space-y-5">
					{/* Model ID + Context Limit */}
					<div className="grid items-start gap-5 sm:grid-cols-2">
						<div className="grid gap-1.5">
							<Label
								htmlFor={modelField.id}
								className="text-sm font-medium text-content-primary"
							>
								Model Identifier{" "}
								<span className="text-xs font-bold text-content-destructive">
									*
								</span>
							</Label>
							<p className="m-0 text-xs text-content-secondary">
								The model identifier sent to the provider API.
							</p>
							<Input
								id={modelField.id}
								name={modelField.name}
								className={cn(
									"h-9 text-[13px] placeholder:text-content-disabled",
									modelField.error && "border-content-destructive",
								)}
								placeholder="e.g. gpt-5, claude-sonnet-4-5"
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
								htmlFor={contextLimitField.id}
								className="text-sm font-medium text-content-primary"
							>
								Context Limit{" "}
								<span className="text-xs font-bold text-content-destructive">
									*
								</span>
							</Label>
							<p className="m-0 text-xs text-content-secondary">
								Max tokens in the context window.
							</p>
							<Input
								id={contextLimitField.id}
								name={contextLimitField.name}
								className={cn(
									"h-9 text-[13px] placeholder:text-content-disabled",
									contextLimitField.error && "border-content-destructive",
								)}
								placeholder="200000"
								value={contextLimitField.value}
								onChange={contextLimitField.onChange}
								onBlur={contextLimitField.onBlur}
								disabled={isSaving}
								aria-invalid={contextLimitField.error}
							/>
							{contextLimitField.error && (
								<p className="m-0 text-xs text-content-destructive">
									{contextLimitField.helperText}
								</p>
							)}
						</div>
					</div>

					{/* Provider-specific model config fields */}
					<ModelConfigFields
						provider={selectedProviderState.provider}
						form={form}
						fieldErrors={modelConfigFormBuildResult.fieldErrors}
						disabled={isSaving}
					/>

					<div className="space-y-5">
						{/* Pricing — toggle */}
						<div>
							<button
								type="button"
								onClick={() => setShowPricing((v) => !v)}
								className="inline-flex cursor-pointer items-center gap-1 bg-transparent border-0 p-0 text-sm font-medium text-content-secondary transition-colors hover:text-content-primary"
							>
								{showPricing ? (
									<ChevronDownIcon className="h-4 w-4" />
								) : (
									<ChevronRightIcon className="h-4 w-4" />
								)}
								Pricing
							</button>
							{showPricing && (
								<div className="mt-4 space-y-3">
									<div>
										<p className="m-0 text-xs text-content-secondary">
											Optional USD pricing metadata per 1M tokens. Leave any
											field blank to keep pricing unset and use provider or
											profile defaults when available.
										</p>
									</div>
									<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
										<PricingModelConfigFields
											provider={selectedProviderState.provider}
											form={form}
											fieldErrors={modelConfigFormBuildResult.fieldErrors}
											disabled={isSaving}
										/>
									</div>
								</div>
							)}
						</div>

						{/* Advanced — toggle */}
						<div>
							<button
								type="button"
								onClick={() => setShowAdvanced((v) => !v)}
								className="inline-flex cursor-pointer items-center gap-1 bg-transparent border-0 p-0 text-sm font-medium text-content-secondary transition-colors hover:text-content-primary"
							>
								{showAdvanced ? (
									<ChevronDownIcon className="h-4 w-4" />
								) : (
									<ChevronRightIcon className="h-4 w-4" />
								)}
								Advanced
							</button>
							{showAdvanced && (
								<div className="mt-4 space-y-5">
									<div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
										<GeneralModelConfigFields
											provider={selectedProviderState.provider}
											form={form}
											fieldErrors={modelConfigFormBuildResult.fieldErrors}
											disabled={isSaving}
										/>
									</div>
									<div className="flex flex-col gap-1.5">
										<Label
											htmlFor={compressionThresholdField.id}
											className="text-sm font-medium text-content-primary"
										>
											Compression Threshold
										</Label>
										<p className="m-0 text-xs text-content-secondary">
											Percentage at which context is compressed.
										</p>
										<Input
											id={compressionThresholdField.id}
											name={compressionThresholdField.name}
											className={cn(
												"h-9 text-[13px] placeholder:text-content-disabled",
												compressionThresholdField.error &&
													"border-content-destructive",
											)}
											placeholder="70"
											value={compressionThresholdField.value}
											onChange={compressionThresholdField.onChange}
											onBlur={compressionThresholdField.onBlur}
											disabled={isSaving}
											aria-invalid={compressionThresholdField.error}
										/>
										{compressionThresholdField.error && (
											<p className="m-0 text-xs text-content-destructive">
												{compressionThresholdField.helperText}
											</p>
										)}
									</div>
								</div>
							)}
						</div>
					</div>
				</div>
				{/* Footer — pushed to bottom */}
				<div className="mt-auto py-6">
					<hr className="mb-4 border-0 border-t border-solid border-border" />
					{confirmingDelete && onDeleteModel && editingModel ? (
						<div className="flex items-center gap-3">
							<p className="m-0 flex-1 text-sm text-content-secondary">
								Are you sure? This action is irreversible.
							</p>
							<div className="flex shrink-0 items-center gap-2">
								<Button
									variant="outline"
									size="lg"
									type="button"
									onClick={() => setConfirmingDelete(false)}
									disabled={isDeleting}
								>
									Cancel
								</Button>
								<Button
									variant="destructive"
									size="lg"
									type="button"
									disabled={isDeleting}
									onClick={() => void onDeleteModel(editingModel.id)}
								>
									{isDeleting && <Spinner className="h-4 w-4" loading />}
									Delete model
								</Button>
							</div>
						</div>
					) : (
						<div className="flex items-center justify-between">
							{isEditing && editingModel && onDeleteModel ? (
								<Button
									variant="outline"
									size="lg"
									type="button"
									className="text-content-secondary hover:text-content-destructive hover:border-border-destructive"
									disabled={isSaving}
									onClick={() => setConfirmingDelete(true)}
								>
									Delete
								</Button>
							) : (
								<Button
									variant="outline"
									size="lg"
									type="button"
									onClick={onCancel}
								>
									Cancel
								</Button>
							)}
							<Button
								size="lg"
								type="submit"
								disabled={isSaving || !form.isValid || hasFieldErrors}
							>
								{isSaving && <Spinner className="h-4 w-4" loading />}{" "}
								{isEditing ? "Save" : "Add model"}
							</Button>
						</div>
					)}
				</div>
			</form>
		</div>
	);
};
