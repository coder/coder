import { useFormik } from "formik";
import { ChevronDownIcon, ChevronRightIcon, InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
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
import { formatProviderLabel } from "../../utils/modelOptions";
import { BackButton } from "../BackButton";
import { ConfirmDeleteDialog } from "../ConfirmDeleteDialog";
import type { ProviderState } from "./ChatModelAdminPanel";
import { normalizeProvider, readOptionalString } from "./helpers";
import {
	GeneralModelConfigFields,
	ModelConfigFields,
	PricingModelConfigFields,
} from "./ModelConfigFields";
import { ModelIdentifierField } from "./ModelIdentifierField";
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
	/** When set without editingModel, the form creates from this model. */
	duplicateSourceModel?: TypesGen.ChatModelConfig;
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
	onToggleEnabled: (
		modelConfigId: string,
		enabled: boolean,
	) => Promise<unknown>;
	onCancel: () => void;
	onDeleteModel?: (modelConfigId: string) => Promise<void>;
}

export const ModelForm: FC<ModelFormProps> = ({
	editingModel,
	duplicateSourceModel,
	providerStates,
	selectedProvider,
	selectedProviderState,
	onSelectedProviderChange,
	modelConfigsUnavailable,
	isSaving,
	isDeleting,
	onCreateModel,
	onUpdateModel,
	onToggleEnabled,
	onCancel,
	onDeleteModel,
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

	// True when the model's original provider no longer exists but a
	// same-type fallback was auto-selected. The alert stays visible
	// until the admin dismisses it or saves the model.
	const originalProviderDeleted =
		isEditing &&
		Boolean(
			editingModel?.ai_provider_id &&
				providerStates.find((ps) => ps.key === editingModel.ai_provider_id) &&
				!providerStates.find((ps) => ps.key === editingModel.ai_provider_id)
					?.providerConfig,
		);
	const [providerAlertDismissed, setProviderAlertDismissed] = useState(false);
	const showProviderAlert = originalProviderDeleted && !providerAlertDismissed;

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			(selectedProviderState.hasEffectiveAPIKey ||
				selectedProviderState.providerConfig.allow_user_api_key),
	);
	const formTitle = isEditing
		? "Edit Model"
		: isDuplicating
			? "Duplicate Model"
			: "Add Model";
	const formDescription = isDuplicating
		? "Review the copied settings, then save to create a new model."
		: undefined;
	const mode: "add" | "edit" | "duplicate" = (() => {
		if (isEditing) return "edit";
		if (isDuplicating) return "duplicate";
		return "add";
	})();

	const selectedProviderType =
		selectedProviderState?.provider ??
		editingModel?.provider ??
		selectedProvider;

	const isProviderMissing =
		isEditing &&
		!selectedProviderState?.providerConfig &&
		!modelConfigsUnavailable;

	const form = useFormik<ModelFormValues>({
		initialValues,
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
				selectedProviderType,
				values.config,
			);
			if (Object.keys(buildResult.fieldErrors).length > 0) return;

			const trimmedDisplayName = values.displayName.trim();
			const builtModelConfig = buildResult.modelConfig;

			const selectedProviderConfigID =
				selectedProviderState?.providerConfig?.id;

			if (isEditing && editingModel) {
				const req: TypesGen.UpdateChatModelConfigRequest = {
					...(selectedProviderConfigID &&
						selectedProviderConfigID !==
							readOptionalString(editingModel.ai_provider_id) && {
							provider: selectedProviderState.provider,
							ai_provider_id: selectedProviderConfigID,
						}),
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
				if (!selectedProvider || !selectedProviderState?.providerConfig) return;

				const req: TypesGen.CreateChatModelConfigRequest = {
					provider: selectedProviderState.provider,
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
			// Navigation is handled by the parent (ModelsSection) after
			// the mutation promise resolves, so we do not call onCancel()
			// here to avoid a double view-transition.
		},
	});

	const getFieldHelpers = getFormHelpers(form);

	const modelConfigFormBuildResult = buildModelConfigFromForm(
		selectedProviderType,
		form.values.config,
	);

	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;
	const defaultModelDisableGuard =
		isEditing && form.values.isDefault && form.values.enabled;

	// ── Provider helpers ──────────────────────────────────────

	const sameTypeProviders = (() => {
		const currentType = (() => {
			if (selectedProviderState?.providerConfig) {
				return normalizeProvider(selectedProviderState.provider);
			}
			if (editingModel) {
				return normalizeProvider(editingModel.provider);
			}
			if (selectedProvider) {
				const found = providerStates.find((ps) => ps.key === selectedProvider);
				return found ? normalizeProvider(found.provider) : "";
			}
			return "";
		})();
		if (!currentType) return providerStates.filter((ps) => ps.providerConfig);
		return providerStates.filter(
			(ps) =>
				ps.providerConfig && normalizeProvider(ps.provider) === currentType,
		);
	})();

	const hasSingleProvider = sameTypeProviders.length <= 1;
	// ── Provider select (shared across all form states) ───────

	const providerSelect = (
		<div className="grid gap-1.5">
			<Label
				htmlFor={
					hasSingleProvider && !isProviderMissing ? undefined : "providerSelect"
				}
				className="text-[13px] font-medium text-content-primary"
			>
				Provider
			</Label>
			{hasSingleProvider && !isProviderMissing ? (
				<Tooltip>
					<TooltipTrigger asChild>
						<div className="flex h-9 items-center gap-2 rounded-md border border-solid border-border px-3 text-[13px] text-content-secondary">
							{selectedProviderState && (
								<ProviderIcon
									provider={selectedProviderState.provider}
									className="size-5 bg-transparent [&>img]:size-full"
								/>
							)}
							{selectedProviderState?.label ?? "No provider"}
						</div>
					</TooltipTrigger>
					<TooltipContent side="bottom" sideOffset={-20}>
						You only have 1{" "}
						{formatProviderLabel(selectedProviderState?.provider ?? "")}{" "}
						provider
					</TooltipContent>
				</Tooltip>
			) : sameTypeProviders.length === 0 ? (
				<div className="flex h-9 items-center rounded-md border border-solid border-border-destructive px-3 text-[13px] text-content-secondary">
					No {formatProviderLabel(editingModel?.provider ?? "")} providers
					available
				</div>
			) : (
				<Select
					value={selectedProvider ?? ""}
					onValueChange={onSelectedProviderChange}
					disabled={providerStates.length === 0}
				>
					<SelectTrigger
						id="providerSelect"
						className={cn(
							"h-9 text-[13px]",
							isProviderMissing && "border-border-destructive",
						)}
					>
						<SelectValue placeholder="Select provider" />
					</SelectTrigger>
					<SelectContent>
						{sameTypeProviders.map((ps) => (
							<SelectItem key={ps.key} value={ps.key}>
								<span className="flex items-center gap-2">
									<ProviderIcon
										provider={ps.provider}
										className="size-4 bg-transparent [&>img]:size-full"
									/>
									{ps.label}
								</span>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			)}
		</div>
	);

	// Skip this guard for orphaned models.
	if (
		(!selectedProviderState && !isProviderMissing) ||
		modelConfigsUnavailable
	) {
		return (
			<div>
				<BackButton onClick={onCancel} />
				<h2 className="m-0 text-lg font-medium text-content-primary">
					{formTitle}
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
				<BackButton onClick={onCancel} />
				<h2 className="m-0 text-lg font-medium text-content-primary">
					{formTitle}
				</h2>
				<hr className="my-4 border-0 border-t border-solid border-border" />
				<div className="space-y-3">
					{providerSelect}
					<p className="text-sm text-content-secondary">
						{!selectedProviderState?.providerConfig
							? "Create a managed provider config on the Providers tab before managing models."
							: "Set an API key for this provider on the Providers tab before managing models."}
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
			<BackButton onClick={onCancel} />
			<div className="mb-4 flex items-center justify-between">
				<div>
					<h2 className="m-0 text-lg font-medium text-content-primary">
						{formTitle}
					</h2>
					{formDescription && (
						<p className="m-0 mt-1 text-sm text-content-secondary">
							{formDescription}
						</p>
					)}
				</div>
				{initialModel && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex">
								<Switch
									checked={form.values.enabled}
									onCheckedChange={(v) => {
										form.setFieldValue("enabled", v);
										if (editingModel) {
											void onToggleEnabled(editingModel.id, v);
										}
										if (v) {
											setProviderAlertDismissed(true);
										}
									}}
									aria-label="Enabled"
									disabled={
										isSaving ||
										defaultModelDisableGuard ||
										(sameTypeProviders.length === 0 && !form.values.enabled)
									}
								/>
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							{defaultModelDisableGuard
								? "Default model cannot be disabled. Remove default status first."
								: sameTypeProviders.length === 0 && !form.values.enabled
									? "No compatible providers available."
									: form.values.enabled
										? "Disable this model. It will be hidden from users."
										: "Enable this model. It will be visible to users."}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			{/* Provider-removed warning */}
			{showProviderAlert && (
				<Alert
					severity="warning"
					className="mb-4"
					dismissible
					onDismiss={() => setProviderAlertDismissed(true)}
				>
					<AlertDescription>
						{sameTypeProviders.length > 0 ? (
							<>
								This model is using a fallback provider. Verify the provider
								selection and re-enable to activate.
							</>
						) : (
							<>
								There are no {formatProviderLabel(editingModel?.provider ?? "")}{" "}
								providers available.{" "}
								<Link
									to="/agents/settings/providers"
									className="text-content-link no-underline hover:underline"
								>
									View providers
								</Link>
							</>
						)}
					</AlertDescription>
				</Alert>
			)}
			<hr className="my-4 border-0 border-t border-solid border-border" />
			{/* Form body */}
			<form
				className="flex flex-1 flex-col"
				onSubmit={form.handleSubmit}
				spellCheck={false}
				autoComplete="off"
			>
				<div className="space-y-6">
					{/* Model name + Provider */}
					<div className="grid items-start gap-4 sm:grid-cols-2">
						<div className="grid gap-1.5">
							<Label
								htmlFor="displayName"
								className="text-[13px] font-medium text-content-primary"
							>
								Model name
							</Label>
							<Input
								id="displayName"
								{...form.getFieldProps("displayName")}
								disabled={isSaving}
								className="h-9 text-[13px]"
								placeholder={initialModel?.model ?? "Model name"}
							/>
						</div>
						{providerSelect}
					</div>

					{/* Model ID + Context Limit + Pricing */}
					<div className="space-y-4">
						<div className="grid items-start gap-4 sm:grid-cols-2">
							{" "}
							<ModelIdentifierField
								form={form}
								modelField={modelField}
								mode={mode}
								selectedProvider={selectedProviderType}
								disabled={isSaving}
							/>
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
											<InfoIcon className="size-3 text-content-secondary" />
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
								<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showPricing && (
							<div className="grid grid-cols-2 gap-3 pt-3 sm:grid-cols-4">
								<PricingModelConfigFields
									provider={selectedProviderType ?? ""}
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
								<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showProviderConfig && (
							<div className="pt-3">
								<ModelConfigFields
									provider={selectedProviderType ?? ""}
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
								<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							) : (
								<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
							)}
						</button>
						{showAdvanced && (
							<div className="grid grid-cols-2 gap-3 pt-3 sm:grid-cols-3">
								<GeneralModelConfigFields
									provider={selectedProviderType ?? ""}
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
												<InfoIcon className="size-3 text-content-secondary" />
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
							disabled={
								isSaving || !form.isValid || hasFieldErrors || isProviderMissing
							}
						>
							{isSaving && <Spinner className="h-4 w-4" loading />}{" "}
							{isEditing
								? "Save"
								: isDuplicating
									? "Create duplicate"
									: "Add model"}
						</Button>
					</div>
				</div>
			</form>
			{editingModel && onDeleteModel && (
				<ConfirmDeleteDialog
					entity="model"
					onConfirm={() => void onDeleteModel(editingModel.id)}
					isPending={isDeleting}
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
				/>
			)}{" "}
		</div>
	);
};
