import { useFormik } from "formik";
import {
	ChevronDownIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	InfoIcon,
	PencilIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { getFormHelpers } from "#/utils/formUtils";
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
	const isDefaultModel = isEditing && editingModel?.is_default === true;
	const [showAdvanced, setShowAdvanced] = useState(false);
	const [showPricing, setShowPricing] = useState(false);
	const [showProviderConfig, setShowProviderConfig] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			(selectedProviderState.hasEffectiveAPIKey ||
				selectedProviderState.providerConfig.allow_user_api_key),
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
					...(values.enabled !== editingModel.enabled && {
						enabled: values.enabled,
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
					enabled: true,
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
	const defaultModelDisableGuard = isDefaultModel && form.values.enabled;

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
			{/* Header - editable display name */}
			<div className="flex items-center gap-3">
				{selectedProviderState && (
					<ProviderIcon
						provider={selectedProviderState.provider}
						className="h-8 w-8"
					/>
				)}
				<div className="inline-flex items-center gap-1">
					<div className="relative inline-grid">
						<span
							className="invisible col-start-1 row-start-1 whitespace-pre text-lg font-medium"
							aria-hidden="true"
						>
							{form.values.displayName ||
								(isEditing
									? (editingModel?.model ?? "Model name")
									: "Model name")}
						</span>
						<input
							type="text"
							{...form.getFieldProps("displayName")}
							disabled={isSaving}
							spellCheck={false}
							className="col-start-1 row-start-1 m-0 min-w-0 border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
							placeholder={
								isEditing ? (editingModel?.model ?? "Model name") : "Model name"
							}
						/>
					</div>
					<PencilIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary" />
				</div>{" "}
				{editingModel && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="ml-auto inline-flex">
								<Switch
									checked={form.values.enabled}
									onCheckedChange={(v) => {
										form.setFieldValue("enabled", v);
									}}
									aria-label="Enabled"
									disabled={isSaving || defaultModelDisableGuard}
								/>
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{defaultModelDisableGuard
								? "Default model cannot be disabled. Remove default status first."
								: form.values.enabled
									? "Disable this model. It will be hidden from users."
									: "Enable this model. It will be visible to users."}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />
			{/* Form body */}
			<form
				className="flex flex-1 flex-col"
				onSubmit={form.handleSubmit}
				spellCheck={false}
				autoComplete="off"
			>
				<div className="space-y-6">
					{/* Model ID + Context Limit + Pricing */}
					<div className="space-y-4">
						<div className="grid items-start gap-4 sm:grid-cols-2">
							{" "}
							<div className="grid gap-1.5">
								<Label
									htmlFor={modelField.id}
									className="inline-flex items-center gap-1 text-sm font-medium text-content-primary"
								>
									Model Identifier{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
									<Tooltip>
										<TooltipTrigger asChild>
											<InfoIcon className="h-3 w-3 text-content-secondary" />
										</TooltipTrigger>
										<TooltipContent side="top" className="max-w-[240px]">
											The model identifier sent to the provider API.
										</TooltipContent>
									</Tooltip>
								</Label>
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
									className="inline-flex items-center gap-1 text-sm font-medium text-content-primary"
								>
									Context Limit{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
									<Tooltip>
										<TooltipTrigger asChild>
											<InfoIcon className="h-3 w-3 text-content-secondary" />
										</TooltipTrigger>
										<TooltipContent side="top" className="max-w-[240px]">
											Max tokens in the context window.
										</TooltipContent>
									</Tooltip>
								</Label>
								<InputGroup
									className={cn(
										"h-9",
										contextLimitField.error && "border-border-destructive",
									)}
								>
									<InputGroupInput
										id={contextLimitField.id}
										name={contextLimitField.name}
										className="h-9 min-w-0 text-[13px] placeholder:text-content-disabled"
										placeholder="200000"
										value={contextLimitField.value}
										onChange={contextLimitField.onChange}
										onBlur={contextLimitField.onBlur}
										disabled={isSaving}
										aria-invalid={contextLimitField.error}
									/>
									<InputGroupAddon align="inline-end">
										<span className="text-xs text-content-disabled">
											tokens
										</span>
									</InputGroupAddon>
								</InputGroup>{" "}
								{contextLimitField.error && (
									<p className="m-0 text-xs text-content-destructive">
										{contextLimitField.helperText}
									</p>
								)}
							</div>
						</div>
					</div>

					{/* Usage Tracking */}
					<div className="border-0 border-t border-solid border-border pt-4">
						<button
							type="button"
							onClick={() => setShowPricing((v) => !v)}
							className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
						>
							<div>
								<h3 className="m-0 text-sm font-medium text-content-primary">
									Cost Tracking{" "}
								</h3>
								<p className="m-0 text-xs text-content-secondary">
									Set per-token pricing so Coder can track costs and enforce
									spending limits.
								</p>
							</div>
							{showPricing ? (
								<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showPricing && (
							<div className="grid grid-cols-2 gap-3 pt-3 sm:grid-cols-4">
								<PricingModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
							</div>
						)}
					</div>

					{/* Provider Configuration */}
					<div className="border-0 border-t border-solid border-border pt-4">
						<button
							type="button"
							onClick={() => setShowProviderConfig((v) => !v)}
							className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
						>
							<div>
								<h3 className="m-0 text-sm font-medium text-content-primary">
									Provider Configuration
								</h3>
								<p className="m-0 text-xs text-content-secondary">
									Tune provider-specific behavior like reasoning, tool calling,
									and web search.
								</p>
							</div>
							{showProviderConfig ? (
								<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showProviderConfig && (
							<div className="pt-3">
								<ModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
							</div>
						)}
					</div>

					{/* Advanced */}
					<div className="border-0 border-t border-solid border-border pt-4">
						<button
							type="button"
							onClick={() => setShowAdvanced((v) => !v)}
							className="flex w-full cursor-pointer items-start justify-between border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary"
						>
							<div>
								<h3 className="m-0 text-sm font-medium text-content-primary">
									Advanced
								</h3>
								<p className="m-0 text-xs text-content-secondary">
									Low-level parameters like temperature and penalties. Rarely
									need changing.
								</p>
							</div>
							{showAdvanced ? (
								<ChevronDownIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 h-4 w-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showAdvanced && (
							<div className="grid grid-cols-2 gap-3 pt-3 sm:grid-cols-3">
								<GeneralModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
								<div className="flex min-w-0 flex-col gap-1.5">
									<Label
										htmlFor={compressionThresholdField.id}
										className="inline-flex items-center gap-1 text-[13px] font-medium text-content-primary"
									>
										Compression Threshold
										<Tooltip>
											<TooltipTrigger asChild>
												<InfoIcon className="h-3 w-3 text-content-secondary" />
											</TooltipTrigger>
											<TooltipContent side="top" className="max-w-[240px]">
												Percentage at which context is compressed.
											</TooltipContent>
										</Tooltip>
									</Label>
									<InputGroup
										className={cn(
											"h-9",
											compressionThresholdField.error &&
												"border-border-destructive",
										)}
									>
										<InputGroupInput
											id={compressionThresholdField.id}
											name={compressionThresholdField.name}
											className="h-9 text-[13px] placeholder:text-content-disabled"
											placeholder="70"
											value={compressionThresholdField.value}
											onChange={compressionThresholdField.onChange}
											onBlur={compressionThresholdField.onBlur}
											disabled={isSaving}
											aria-invalid={compressionThresholdField.error}
										/>
										<InputGroupAddon align="inline-end">
											<span className="text-xs text-content-disabled">%</span>
										</InputGroupAddon>
									</InputGroup>
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
				<div className="mt-auto py-6">
					<hr className="mb-4 border-0 border-t border-solid border-border" />
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
				</div>
			</form>
			{editingModel && onDeleteModel && (
				<Dialog
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
				>
					<DialogContent variant="destructive">
						<DialogHeader>
							<DialogTitle>Delete model</DialogTitle>
							<DialogDescription>
								Are you sure you want to delete this model? This action is
								irreversible.
							</DialogDescription>
						</DialogHeader>
						<DialogFooter>
							<Button
								variant="outline"
								onClick={() => setConfirmingDelete(false)}
								disabled={isDeleting}
							>
								Cancel
							</Button>
							<Button
								variant="destructive"
								onClick={() => void onDeleteModel(editingModel.id)}
								disabled={isDeleting}
							>
								{isDeleting && <Spinner className="h-4 w-4" loading />}
								Delete model
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}{" "}
		</div>
	);
};
