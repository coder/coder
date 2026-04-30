import type { FormikContextType } from "formik";
import { CheckIcon, InfoIcon } from "lucide-react";
import { type FocusEvent, useRef, useState } from "react";
import { Autocomplete } from "#/components/Autocomplete/Autocomplete";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";
import { normalizeProvider } from "./helpers";
import {
	findKnownModelByCanonicalId,
	formatContextBadge,
	getKnownModelsForProvider,
	type KnownModel,
	searchKnownModels,
} from "./knownModels";
import { applyKnownModelDefaults } from "./knownModels/applyKnownModelDefaults";
import type { ModelFormValues } from "./modelConfigFormLogic";

type ModelFormMode = "add" | "edit" | "duplicate";

type ModelIdentifierFieldProps = {
	form: FormikContextType<ModelFormValues>;
	modelField: FormHelpers;
	mode: ModelFormMode;
	selectedProvider: string | null;
	disabled: boolean;
};

type ModelIdentifierOption = {
	model: string;
	displayName: string;
	contextLimit?: number;
	knownModel?: KnownModel;
};

type AppliedModel = {
	provider: string;
	model: string;
};

const knownModelToOption = (knownModel: KnownModel): ModelIdentifierOption => ({
	model: knownModel.modelIdentifier,
	displayName: knownModel.displayName,
	contextLimit: knownModel.contextLimit,
	knownModel,
});

export const ModelIdentifierField = ({
	form,
	modelField,
	mode,
	selectedProvider,
	disabled,
}: ModelIdentifierFieldProps) => {
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);
	const [feedback, setFeedback] = useState<string | null>(null);
	const initialValuesRef = useRef<ModelFormValues | null>(null);
	const lastAppliedProviderModelRef = useRef<AppliedModel | null>(null);

	if (initialValuesRef.current === null) {
		// ModelsSection remounts this form when the add-mode provider changes, so
		// this snapshot is the safety baseline for advisory default application.
		initialValuesRef.current = form.initialValues;
	}

	const normalizedProvider = normalizeProvider(selectedProvider ?? "");
	const providerKnownModels = getKnownModelsForProvider(normalizedProvider);
	const usesKnownModelCatalog =
		mode === "add" &&
		(normalizedProvider === "openai" || normalizedProvider === "anthropic") &&
		providerKnownModels.length > 0;

	const markTouched = () => {
		void form.setFieldTouched("model", true);
	};

	const renderPlainInput = () => (
		<div className="grid gap-1.5">
			<Label
				htmlFor={modelField.id}
				className="inline-flex items-center gap-1 text-sm font-medium text-content-primary"
			>
				Model Identifier{" "}
				<span className="text-xs font-bold text-content-destructive">*</span>
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
				disabled={disabled}
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
	);

	if (!usesKnownModelCatalog) {
		return renderPlainInput();
	}

	const knownModelOptions = (
		inputValue.trim() === ""
			? providerKnownModels
			: searchKnownModels(normalizedProvider, inputValue)
	).map(knownModelToOption);
	const currentModel = String(form.values.model ?? "");
	const selectedKnownModel = findKnownModelByCanonicalId(
		normalizedProvider,
		currentModel,
	);
	const selectedOption = selectedKnownModel
		? knownModelToOption(selectedKnownModel)
		: currentModel
			? { model: currentModel, displayName: currentModel }
			: null;

	const applyDefaultsForKnownModel = (knownModel: KnownModel) => {
		if (knownModel.provider !== normalizedProvider) {
			return;
		}

		const initialValues = initialValuesRef.current;
		if (initialValues === null) {
			return;
		}

		const nextValuesForHelper = {
			...form.values,
			model: knownModel.modelIdentifier,
		};
		const result = applyKnownModelDefaults({
			values: nextValuesForHelper,
			initialValues,
			provider: normalizedProvider,
			knownModel,
		});
		void form.setValues(result.values);
		// Selecting and blurring can both observe the same canonical model. This
		// ref skips the repeat apply so feedback does not flicker or duplicate.
		lastAppliedProviderModelRef.current = {
			provider: normalizedProvider,
			model: knownModel.modelIdentifier,
		};
		setFeedback(
			result.appliedFields.length > 0
				? `Defaults applied from ${knownModel.displayName}. Review and adjust before saving.`
				: null,
		);
	};

	const applyDefaultsOnExactCanonicalBlur = () => {
		const found = findKnownModelByCanonicalId(normalizedProvider, currentModel);
		if (!found) {
			return;
		}

		const lastApplied = lastAppliedProviderModelRef.current;
		if (
			lastApplied?.provider === normalizedProvider &&
			lastApplied.model === found.modelIdentifier
		) {
			return;
		}

		// Exact canonical-id blur is safe because the submitted value already
		// matches the catalog id. Aliases stay free text and do not apply defaults.
		applyDefaultsForKnownModel(found);
	};

	const handleChange = (option: ModelIdentifierOption | null) => {
		if (!option) {
			void form.setFieldValue("model", "");
			setInputValue("");
			setFeedback(null);
			lastAppliedProviderModelRef.current = null;
			return;
		}

		if (!option.knownModel) {
			void form.setFieldValue("model", option.model);
			setFeedback(null);
			lastAppliedProviderModelRef.current = null;
			return;
		}

		applyDefaultsForKnownModel(option.knownModel);
		setInputValue("");
	};

	const handleInputChange = (value: string) => {
		setInputValue(value);
		setFeedback(null);
		lastAppliedProviderModelRef.current = null;
		void form.setFieldValue("model", value);
	};

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (nextOpen) {
			return;
		}
		markTouched();
		setInputValue("");
		applyDefaultsOnExactCanonicalBlur();
	};

	const handleBlur = (event: FocusEvent<HTMLDivElement>) => {
		const relatedTarget = event.relatedTarget;
		if (
			relatedTarget instanceof Node &&
			event.currentTarget.contains(relatedTarget)
		) {
			return;
		}
		markTouched();
		applyDefaultsOnExactCanonicalBlur();
	};

	return (
		<div className="grid gap-1.5" onBlur={handleBlur}>
			<Label
				htmlFor={modelField.id}
				className="inline-flex items-center gap-1 text-sm font-medium text-content-primary"
			>
				Model Identifier{" "}
				<span className="text-xs font-bold text-content-destructive">*</span>
				<Tooltip>
					<TooltipTrigger asChild>
						<InfoIcon className="h-3 w-3 text-content-secondary" />
					</TooltipTrigger>
					<TooltipContent side="top" className="max-w-[240px]">
						The model identifier sent to the provider API.
					</TooltipContent>
				</Tooltip>
			</Label>
			<Autocomplete
				id={modelField.id}
				value={selectedOption}
				onChange={handleChange}
				options={knownModelOptions}
				getOptionValue={(option) => option.model}
				getOptionLabel={(option) => option.model}
				isOptionEqualToValue={(option, value) => option.model === value.model}
				renderOption={(option, isSelected) => (
					<div className="flex w-full min-w-0 items-center justify-between gap-3">
						<div className="min-w-0">
							<div className="truncate text-sm text-content-primary">
								{option.displayName}
							</div>
							<div className="truncate text-xs text-content-secondary">
								{option.model}
							</div>
						</div>
						<div className="flex shrink-0 items-center gap-2 text-xs text-content-secondary">
							{option.contextLimit !== undefined && (
								<span>{formatContextBadge(option.contextLimit)}</span>
							)}
							{isSelected && <CheckIcon className="size-4 shrink-0" />}
						</div>
					</div>
				)}
				open={open}
				onOpenChange={handleOpenChange}
				inputValue={inputValue}
				onInputChange={handleInputChange}
				placeholder="e.g. gpt-5, claude-sonnet-4-5"
				noOptionsText="No known models found"
				className={cn(
					"h-9 text-[13px] placeholder:text-content-disabled",
					modelField.error && "border-content-destructive",
				)}
				disabled={disabled}
			/>
			{modelField.error && (
				<p
					id={`${modelField.id}-error`}
					className="m-0 text-xs text-content-destructive"
				>
					{modelField.helperText}
				</p>
			)}
			{feedback && (
				<p
					className="m-0 text-xs text-content-secondary"
					role="status"
					aria-live="polite"
				>
					{feedback}
				</p>
			)}
		</div>
	);
};
