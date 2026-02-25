import { getErrorMessage } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Input } from "components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { ArrowLeftIcon, Loader2Icon, PlusIcon, SaveIcon } from "lucide-react";
import {
	type FC,
	type FormEvent,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";
import { cn } from "utils/cn";
import type { ProviderState } from "./ChatModelAdminPanel";
import { ModelConfigFields } from "./ModelConfigFields";
import { ProviderIcon } from "./ProviderIcon";
import {
	type ModelConfigFormState,
	buildModelConfigFromForm,
	emptyModelConfigFormState,
	extractModelConfigFormState,
	parsePositiveInteger,
	parseThresholdInteger,
} from "./modelConfigFormLogic";
import { getModelConfigSchemaReference } from "./modelConfigSchemas";

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

	const providerSelectInputId = useId();
	const modelInputId = useId();
	const displayNameInputId = useId();
	const contextLimitInputId = useId();
	const compressionThresholdInputId = useId();
	const modelConfigInputId = useId();

	const [model, setModel] = useState(editingModel?.model ?? "");
	const [displayName, setDisplayName] = useState(
		editingModel?.display_name ?? "",
	);
	const [contextLimit, setContextLimit] = useState(
		editingModel ? String(editingModel.context_limit) : "",
	);
	const [compressionThreshold, setCompressionThreshold] = useState(
		editingModel ? String(editingModel.compression_threshold) : "70",
	);
	const [modelConfigForm, setModelConfigForm] = useState<ModelConfigFormState>(
		() =>
			editingModel
				? extractModelConfigFormState(editingModel)
				: { ...emptyModelConfigFormState },
	);

	// Reset form fields when the selected provider changes (add
	// mode only — in edit mode the provider is fixed).
	useEffect(() => {
		if (!isEditing) {
			setModelConfigForm({ ...emptyModelConfigFormState });
		}
	}, [isEditing, selectedProviderState?.provider]);

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
	);

	const modelConfigFormBuildResult = useMemo(
		() =>
			buildModelConfigFromForm(
				selectedProviderState?.provider,
				modelConfigForm,
			),
		[selectedProviderState?.provider, modelConfigForm],
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
			<label
				htmlFor={providerSelectInputId}
				className="text-[13px] font-medium text-content-primary"
			>
				Provider
			</label>
			<Select
				value={selectedProvider ?? ""}
				onValueChange={onSelectedProviderChange}
				disabled={isEditing || providerStates.length === 0}
			>
				<SelectTrigger
					id={providerSelectInputId}
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

	const parsedContextLimit = parsePositiveInteger(contextLimit);
	const parsedCompressionThreshold =
		parseThresholdInteger(compressionThreshold);
	const contextLimitError =
		contextLimit.trim() && parsedContextLimit === null
			? "Context limit must be a positive integer."
			: undefined;
	const compressionThresholdError =
		compressionThreshold.trim() && parsedCompressionThreshold === null
			? "Compression threshold must be a number between 0 and 100."
			: undefined;

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (isSaving) return;

		const trimmedModel = model.trim();
		if (!trimmedModel) return;
		if (parsedContextLimit === null) return;
		if (parsedCompressionThreshold === null) return;
		if (hasFieldErrors) return;

		const trimmedDisplayName = displayName.trim();
		const builtModelConfig = modelConfigFormBuildResult.modelConfig;

		try {
			if (isEditing && editingModel) {
				const req: TypesGen.UpdateChatModelConfigRequest = {
					...(trimmedModel !== editingModel.model && {
						model: trimmedModel,
					}),
					...(trimmedDisplayName !== (editingModel.display_name ?? "") && {
						display_name: trimmedDisplayName,
					}),
					...(parsedContextLimit !== editingModel.context_limit && {
						context_limit: parsedContextLimit,
					}),
					...(parsedCompressionThreshold !==
						editingModel.compression_threshold && {
						compression_threshold: parsedCompressionThreshold,
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
					context_limit: parsedContextLimit,
					compression_threshold: parsedCompressionThreshold,
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
		} catch (error) {
			displayError(
				getErrorMessage(error, "Failed to save model configuration."),
			);
		}
	};

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
				onSubmit={(event) => void handleSubmit(event)}
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
						<div className="grid gap-3 md:grid-cols-3">
							{providerSelect}
							<div className="grid gap-1.5">
								<label
									htmlFor={modelInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Model ID
								</label>
								<Input
									id={modelInputId}
									className="h-10 text-[13px]"
									placeholder="gpt-5, claude-sonnet-4-5, etc."
									value={model}
									onChange={(e) => setModel(e.target.value)}
									disabled={isSaving}
								/>
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={displayNameInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Display name{" "}
									<span className="font-normal text-content-secondary">
										(optional)
									</span>
								</label>
								<Input
									id={displayNameInputId}
									className="h-10 text-[13px]"
									placeholder="Friendly label"
									value={displayName}
									onChange={(e) => setDisplayName(e.target.value)}
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
								Leave values blank to use backend defaults.
							</p>
						</div>
						<div className="grid gap-3 md:grid-cols-2">
							<div className="grid gap-1.5">
								<label
									htmlFor={contextLimitInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Context limit
								</label>
								<Input
									id={contextLimitInputId}
									className={cn(
										"h-10 text-[13px]",
										contextLimitError && "border-content-destructive",
									)}
									placeholder="200000"
									value={contextLimit}
									onChange={(e) => setContextLimit(e.target.value)}
									disabled={isSaving}
								/>
								{contextLimitError && (
									<p className="m-0 text-xs text-content-destructive">
										{contextLimitError}
									</p>
								)}
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={compressionThresholdInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Compression threshold
								</label>
								<Input
									id={compressionThresholdInputId}
									className={cn(
										"h-10 text-[13px]",
										compressionThresholdError && "border-content-destructive",
									)}
									placeholder="70"
									value={compressionThreshold}
									onChange={(e) => setCompressionThreshold(e.target.value)}
									disabled={isSaving}
								/>
								{compressionThresholdError && (
									<p className="m-0 text-xs text-content-destructive">
										{compressionThresholdError}
									</p>
								)}
							</div>
						</div>
					</div>

					{/* Model call config fields */}
					<ModelConfigFields
						provider={selectedProviderState.provider}
						form={modelConfigForm}
						fieldErrors={modelConfigFormBuildResult.fieldErrors}
						onChange={(key, value) =>
							setModelConfigForm((prev) => ({
								...prev,
								[key]: value,
							}))
						}
						disabled={isSaving}
						inputIdPrefix={modelConfigInputId}
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
						disabled={
							isSaving ||
							!model.trim() ||
							parsedContextLimit === null ||
							parsedCompressionThreshold === null ||
							hasFieldErrors
						}
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
