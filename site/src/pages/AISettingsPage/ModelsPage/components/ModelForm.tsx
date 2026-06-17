import { useFormik } from "formik";
import {
	ArrowLeftIcon,
	ChevronDownIcon,
	ChevronRightIcon,
	CopyIcon,
	EllipsisVerticalIcon,
	InfoIcon,
	TrashIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { type FC, type ReactNode, useRef, useState } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import {
	canManageProviderModels,
	type ProviderState,
} from "#/modules/aiModels/providerStates";
import { readOptionalString } from "#/pages/AgentsPage/components/ChatModelAdminPanel/helpers";
import {
	GeneralModelConfigFields,
	ModelConfigFields,
	PricingModelConfigFields,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelConfigFields";
import { ModelIdentifierField } from "#/pages/AgentsPage/components/ChatModelAdminPanel/ModelIdentifierField";
import {
	buildInitialModelFormValues,
	buildModelConfigFromForm,
	type ModelFormValues,
	parsePositiveInteger,
	parseThresholdInteger,
} from "#/pages/AgentsPage/components/ChatModelAdminPanel/modelConfigFormLogic";
import { ConfirmDeleteDialog } from "#/pages/AgentsPage/components/ConfirmDeleteDialog";
import {
	getProviderIcon,
	ProviderIcon,
} from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { cn } from "#/utils/cn";
import { getFormHelpers } from "#/utils/formUtils";
import { indefiniteArticle } from "#/utils/text";

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

const CollapsibleSection: FC<{
	title: string;
	description: string;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	className?: string;
	contentClassName?: string;
	children: ReactNode;
}> = ({
	title,
	description,
	open,
	onOpenChange,
	className,
	contentClassName,
	children,
}) => {
	return (
		<Collapsible
			open={open}
			onOpenChange={onOpenChange}
			className={cn("p-4", className)}
		>
			<CollapsibleTrigger className="flex w-full cursor-pointer items-start gap-2 border-0 bg-transparent p-0 text-left transition-colors hover:text-content-primary">
				{open ? (
					<ChevronDownIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
				) : (
					<ChevronRightIcon className="mt-0.5 size-4 shrink-0 text-content-secondary" />
				)}
				<div>
					<h3 className="m-0 text-sm font-medium text-content-primary">
						{title}
					</h3>
					<p className="m-0 text-xs text-content-secondary">{description}</p>
				</div>
			</CollapsibleTrigger>
			{/* Layout classes live on an inner wrapper so a `display` utility
				   like `grid` doesn't override the collapsed `[hidden]` state and
			leave the padding occupying space. */}
			<CollapsibleContent>
				<div className={contentClassName}>{children}</div>
			</CollapsibleContent>
		</Collapsible>
	);
};

interface ModelFormProps {
	/** When set, the form is in "edit" mode for the given model. */
	editingModel?: TypesGen.ChatModelConfig;
	/** When set without editingModel, the form creates from this model. */
	duplicateSourceModel?: TypesGen.ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProviderState: ProviderState | null;
	/** Switches the provider in add mode (typically navigates). */
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
	/**
	 * The model that is currently the deployment default, if any. Used to warn
	 * before replacing it when setting a different model as the default.
	 */
	currentDefaultModel?: TypesGen.ChatModelConfig;
	onSetDefault?: () => void;
	onDuplicate?: () => void;
	/** Persists the enabled state immediately, like the Providers page. */
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
	// Set once the replace-default warning has been acknowledged so the next
	// submit goes through instead of re-opening the dialog.
	const replaceDefaultConfirmedRef = useRef(false);

	const canManageModels = canManageProviderModels(
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

			// Warn before stealing the default from another model. Resolved by the
			// replace-default dialog, which re-submits with the ref set.
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

	// Warn before leaving with unsaved edits. The blocker intercepts the
	// navigation triggered by the Back/Cancel links (and browser nav); guard on
	// isSubmitting so the post-save redirect isn't itself blocked.
	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	const modelConfigFormBuildResult = buildModelConfigFromForm(
		selectedProviderType,
		form.values.config,
	);
	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;
	// A default model must stay enabled, so its toggle is locked until another
	// model is made the default. Driven by the live model, not form state,
	// because enabling/disabling persists immediately rather than on submit.
	const enabledToggleDisabled =
		isSaving ||
		(editingModel?.is_default === true && editingModel.enabled === true);
	// In edit mode the default can't be cleared from here (set another model as
	// default instead), and a disabled model can't be made the default.
	const setDefaultDisabled =
		isSaving ||
		(isEditing &&
			(editingModel?.is_default === true || editingModel?.enabled === false));

	// Derive the submit-disabled state synchronously from the current values.
	// Relying on Formik's `isValid` races with `validateOnMount`, whose errors
	// only populate in a post-render effect, leaving the button briefly enabled.
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
		compressionThresholdValid;

	const handleConfirmReplaceDefault = () => {
		replaceDefaultConfirmedRef.current = true;
		setConfirmingReplaceDefault(false);
		void form.submitForm();
	};

	const backLink = (
		<Link to="/ai/settings/models" className="-ml-3">
			<Button variant="subtle" type="button">
				<ArrowLeftIcon />
				<span>Back to models</span>
			</Button>
		</Link>
	);

	const providerSelect = (
		<div className="grid gap-1.5">
			<Label
				htmlFor="providerSelect"
				className="flex items-center gap-1 leading-6 text-content-primary"
			>
				Provider{" "}
				<span className="text-xs font-bold text-content-destructive">*</span>
			</Label>
			<p className="m-0 text-xs text-content-secondary">
				The provider this model belongs to.
			</p>
			<Select
				value={selectedProviderKey}
				onValueChange={onProviderChange}
				disabled={isDuplicating || providerStates.length === 0}
			>
				<SelectTrigger id="providerSelect" className="text-content-primary">
					<SelectValue placeholder="Select provider" />
				</SelectTrigger>
				<SelectContent>
					{providerStates.map((ps) => (
						<SelectItem key={ps.key} value={ps.key}>
							<span className="flex items-center gap-2">
								<ProviderIcon provider={ps.provider} />
								{ps.label}
							</span>
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);

	// No provider selected or the provider cannot manage models on add.
	if (!selectedProviderState || (!canManageModels && !isEditing)) {
		return (
			<>
				{backLink}
				<div className="flex flex-col gap-6 pt-6">
					<SettingsHeaderTitle>Add model</SettingsHeaderTitle>
					<div className="border border-solid p-6 rounded-lg">
						<div className="space-y-3">
							{providerSelect}
							{selectedProviderState && (
								<p className="text-sm text-content-secondary m-0">
									{!selectedProviderState.providerConfig
										? "Create a managed provider before managing models."
										: "Set an API key for this provider before managing models."}
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
			<div className="flex items-center justify-between">
				{backLink}
				{isEditing && editingModel && onDeleteModel && (
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="subtle"
								size="icon"
								type="button"
								disabled={isSaving}
								aria-label="Model actions"
							>
								<EllipsisVerticalIcon />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							{onDuplicate && (
								<DropdownMenuItem onClick={onDuplicate}>
									<CopyIcon className="size-icon-sm" />
									Duplicate model
								</DropdownMenuItem>
							)}
							<DropdownMenuSeparator />
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onClick={() => setConfirmingDelete(true)}
							>
								<TrashIcon />
								Delete…
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				)}
			</div>
			<div className="flex flex-col gap-6 pt-6">
				<div className="flex items-center justify-between gap-4">
					<div className="flex items-center gap-4 min-w-0">
						<Avatar
							variant="icon"
							size="lg"
							src={getProviderIcon(selectedProviderState.provider)}
						/>
						<SettingsHeaderTitle>
							<span
								className={cn(
									"block min-w-0 truncate",
									editingModel?.enabled === false && "text-content-secondary",
								)}
							>
								{title}
							</span>
						</SettingsHeaderTitle>
						{isEditing && editingModel?.is_default && (
							<Badge variant="default">Default</Badge>
						)}
						{isEditing &&
							editingModel &&
							!editingModel.is_default &&
							!editingModel.enabled && (
								<Badge variant="default">Disabled</Badge>
							)}
					</div>
					{isEditing && editingModel && (
						<div className="flex shrink-0 items-center gap-2">
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="inline-flex">
										<Switch
											checked={editingModel.enabled}
											onCheckedChange={(checked) => onToggleEnabled?.(checked)}
											disabled={enabledToggleDisabled}
											aria-label="Model enabled"
										/>
									</span>
								</TooltipTrigger>
								<TooltipContent side="bottom">
									{editingModel.is_default && editingModel.enabled
										? "Default model cannot be disabled. Set another model as default first."
										: editingModel.enabled
											? "Disable this model. It will be hidden from users."
											: "Enable this model. It will be visible to users."}
								</TooltipContent>
							</Tooltip>
							<span className="text-sm">Enable</span>
						</div>
					)}
				</div>
				<div className="border border-solid p-6 rounded-lg">
					<form
						onSubmit={form.handleSubmit}
						spellCheck={false}
						autoComplete="off"
						className="flex flex-col gap-6"
					>
						<div className="grid items-start gap-4 sm:grid-cols-2">
							{providerSelect}
							<div className="flex flex-col gap-1">
								<ModelIdentifierField
									form={form}
									modelField={modelField}
									mode={mode}
									selectedProvider={selectedProviderType}
									disabled={isSaving}
								/>
								<label
									htmlFor="isDefault"
									className="flex w-fit cursor-pointer items-center gap-2 font-normal text-sm leading-6 text-content-secondary"
								>
									<Checkbox
										id="isDefault"
										checked={form.values.isDefault}
										onCheckedChange={(checked) =>
											form.setFieldValue("isDefault", checked === true)
										}
										disabled={setDefaultDisabled}
									/>
									Set as default model
								</label>
							</div>
							<div className="grid gap-1.5">
								<Label
									htmlFor={displayNameField.id}
									className="flex items-center gap-1 leading-6 text-content-primary"
								>
									Display name{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
								</Label>
								<p className="m-0 text-xs text-content-secondary">
									Friendly name. Defaults to identifier if blank.
								</p>
								<Input
									id={displayNameField.id}
									name={displayNameField.name}
									className="placeholder:text-content-disabled"
									placeholder={initialModel?.model ?? "Model name"}
									value={displayNameField.value}
									onChange={displayNameField.onChange}
									onBlur={displayNameField.onBlur}
									disabled={isSaving}
								/>
							</div>
							<div className="grid gap-1.5">
								<Label
									htmlFor={contextLimitField.id}
									className="flex items-center gap-1 leading-6 text-content-primary"
								>
									Context limit{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
								</Label>
								{contextLimitField.error ? (
									<p className="m-0 text-xs text-content-destructive">
										{contextLimitField.helperText}
									</p>
								) : (
									<p className="m-0 text-xs text-content-secondary">
										Max tokens in the context window.
									</p>
								)}
								<InputGroup
									className={cn(
										contextLimitField.error && "border-border-destructive",
									)}
								>
									<InputGroupInput
										id={contextLimitField.id}
										name={contextLimitField.name}
										className="min-w-0 placeholder:text-content-disabled"
										placeholder="200000"
										value={contextLimitField.value}
										onChange={contextLimitField.onChange}
										onBlur={contextLimitField.onBlur}
										disabled={isSaving}
										aria-invalid={contextLimitField.error}
									/>
									<InputGroupAddon align="inline-end">
										<span className="text-xs text-content-disabled">
											Tokens
										</span>
									</InputGroupAddon>
								</InputGroup>
							</div>
						</div>

						{/* Collapsible sections share one enclosing rounded panel. */}
						<div className="overflow-hidden rounded-lg border border-solid border-border">
							{/* Cost tracking */}
							<CollapsibleSection
								title="Cost tracking"
								description="Set per-token pricing so Coder can track costs and enforce spending limits."
								open={showPricing}
								onOpenChange={setShowPricing}
								contentClassName="grid grid-cols-2 gap-3 pt-3 pl-6 sm:grid-cols-4"
							>
								<PricingModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
							</CollapsibleSection>

							{/* Provider configuration */}
							<CollapsibleSection
								title="Provider configuration"
								description="Tune provider-specific behavior like reasoning, tool calling, and web search."
								open={showProviderConfig}
								onOpenChange={setShowProviderConfig}
								className="border-0 border-t border-solid border-border"
								contentClassName="pt-3 pl-6"
							>
								<ModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
							</CollapsibleSection>

							{/* Advanced */}
							<CollapsibleSection
								title="Advanced"
								description="Low-level parameters like temperature and penalties. Rarely need changing."
								open={showAdvanced}
								onOpenChange={setShowAdvanced}
								className="border-0 border-t border-solid border-border"
								contentClassName="grid grid-cols-2 gap-3 pt-3 pl-6 sm:grid-cols-3"
							>
								<GeneralModelConfigFields
									provider={selectedProviderState.provider}
									form={form}
									fieldErrors={modelConfigFormBuildResult.fieldErrors}
									disabled={isSaving}
								/>
								<div className="flex min-w-0 flex-col gap-1.5">
									<Label
										htmlFor={compressionThresholdField.id}
										className="flex items-center gap-1 leading-6 text-content-primary"
									>
										Compression threshold
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
											compressionThresholdField.error &&
												"border-border-destructive",
										)}
									>
										<InputGroupInput
											id={compressionThresholdField.id}
											name={compressionThresholdField.name}
											className="placeholder:text-content-disabled"
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
							</CollapsibleSection>
						</div>

						{/* Footer */}
						<div className="flex items-center justify-end gap-3">
							<Link to="/ai/settings/models">
								<Button variant="outline" type="button">
									Cancel
								</Button>
							</Link>
							<Button type="submit" disabled={!canSubmit}>
								{isSaving && <Spinner loading />}
								{isEditing
									? "Update model"
									: isDuplicating
										? "Create duplicate"
										: "Add Model"}
							</Button>
						</div>
					</form>
				</div>
			</div>
			{editingModel && onDeleteModel && (
				<ConfirmDeleteDialog
					entity="model"
					isPending={isDeleting}
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
					onConfirm={() => {
						// Deleting removes the model, so drop the dirty guard to keep the
						// post-delete redirect from tripping the unsaved-changes prompt.
						form.resetForm({ values: form.values });
						void onDeleteModel(editingModel.id);
					}}
				/>
			)}
			<Dialog
				open={unsavedChanges.isOpen}
				onOpenChange={(open) => !open && unsavedChanges.onCancel()}
			>
				<DialogContent variant="warning">
					<DialogHeader>
						<DialogTitle>Unsaved changes</DialogTitle>
						<DialogDescription className="flex items-start gap-3">
							<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0 text-content-primary" />
							<span>Your updates haven't been saved. Leave anyway?</span>
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="outline"
							type="button"
							onClick={unsavedChanges.onCancel}
						>
							Cancel
						</Button>
						<Button type="button" onClick={unsavedChanges.onConfirm}>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
			<Dialog
				open={confirmingReplaceDefault}
				onOpenChange={(open) => !open && setConfirmingReplaceDefault(false)}
			>
				<DialogContent variant="warning">
					<DialogHeader>
						<DialogTitle>Replace default model</DialogTitle>
						<DialogDescription className="flex items-center gap-2">
							<TriangleAlertIcon className="size-icon-sm shrink-0 text-content-primary" />
							<span>
								<strong className="text-content-primary">
									{currentDefaultModel?.display_name ||
										currentDefaultModel?.model}
								</strong>{" "}
								is currently the default. Replace it?
							</span>
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<Button
							variant="outline"
							type="button"
							onClick={() => setConfirmingReplaceDefault(false)}
						>
							Cancel
						</Button>
						<Button type="button" onClick={handleConfirmReplaceDefault}>
							Confirm
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};
