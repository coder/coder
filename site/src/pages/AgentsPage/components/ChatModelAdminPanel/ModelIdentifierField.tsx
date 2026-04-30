import type { FormikContextType } from "formik";
import { CheckIcon, InfoIcon } from "lucide-react";
import {
	type FocusEvent,
	type KeyboardEvent,
	useEffect,
	useRef,
	useState,
} from "react";
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
	findKnownModelByAlias,
	findKnownModelByCanonicalId,
	formatContextBadge,
	getKnownModelsForProvider,
	type KnownModel,
	searchKnownModels,
} from "./knownModels";
import { applyKnownModelDefaults } from "./knownModels/applyKnownModelDefaults";
import { deepGet, deepSet, type ModelFormValues } from "./modelConfigFormLogic";

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
	modelIdentifier: string;
};

type PreviouslyAppliedDefaults = {
	provider: string;
	modelIdentifier: string;
	fields: Record<string, unknown>;
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
	const [initialFormValues] = useState(() => form.initialValues);
	const [open, setOpen] = useState(false);
	const [searchValue, setSearchValue] = useState("");
	const [feedback, setFeedback] = useState<string | null>(null);
	const searchValueRef = useRef("");
	const searchDirtyRef = useRef(false);
	const justSelectedRef = useRef(false);
	const closeIntentRef = useRef<"escape" | null>(null);
	const lastAppliedProviderModelRef = useRef<AppliedModel | null>(null);
	const previouslyAppliedRef = useRef<PreviouslyAppliedDefaults | null>(null);

	const normalizedProvider = normalizeProvider(selectedProvider ?? "");
	const providerKnownModels = getKnownModelsForProvider(normalizedProvider);
	const usesKnownModelCatalog =
		mode === "add" && providerKnownModels.length > 0;
	const currentModel = String(form.values.model ?? "");
	const activeSearchQuery = open ? searchValue : currentModel;
	const knownModelOptions = (
		activeSearchQuery.trim() === ""
			? providerKnownModels
			: searchKnownModels(normalizedProvider, activeSearchQuery)
	).map(knownModelToOption);
	const selectedKnownModel = findKnownModelByCanonicalId(
		normalizedProvider,
		currentModel,
	);
	const selectedOption = selectedKnownModel
		? knownModelToOption(selectedKnownModel)
		: currentModel
			? { model: currentModel, displayName: currentModel }
			: null;
	const hasError = Boolean(modelField.error);
	const errorId = hasError ? `${modelField.id}-error` : undefined;

	useEffect(() => {
		// This reset is keyed to provider changes even though the reset logic
		// does not need the provider value itself.
		void normalizedProvider;
		setFeedback(null);
		lastAppliedProviderModelRef.current = null;
		justSelectedRef.current = false;
		closeIntentRef.current = null;
		setSearchValue("");
		searchValueRef.current = "";
		searchDirtyRef.current = false;
		previouslyAppliedRef.current = null;
	}, [normalizedProvider]);

	useEffect(() => {
		if (!usesKnownModelCatalog || !open) {
			return;
		}

		const markEscapeCloseIntent = (event: globalThis.KeyboardEvent) => {
			if (event.key === "Escape") {
				closeIntentRef.current = "escape";
			}
		};

		window.addEventListener("keydown", markEscapeCloseIntent, true);
		document.addEventListener("keydown", markEscapeCloseIntent, true);
		return () => {
			window.removeEventListener("keydown", markEscapeCloseIntent, true);
			document.removeEventListener("keydown", markEscapeCloseIntent, true);
		};
	}, [open, usesKnownModelCatalog]);

	const markTouched = () => {
		void form.setFieldTouched("model", true);
	};

	const setSearchSnapshot = (value: string) => {
		setSearchValue(value);
		searchValueRef.current = value;
	};

	const clearSearchSnapshot = () => {
		setSearchSnapshot("");
		searchDirtyRef.current = false;
	};

	const clearAppliedModelState = () => {
		setFeedback(null);
		lastAppliedProviderModelRef.current = null;
		previouslyAppliedRef.current = null;
	};

	const applyDefaultsForKnownModel = (knownModel: KnownModel) => {
		if (knownModel.provider !== normalizedProvider) {
			return;
		}

		let effectiveInitialValues = initialFormValues;
		const previouslyApplied = previouslyAppliedRef.current;
		if (previouslyApplied?.provider === normalizedProvider) {
			effectiveInitialValues = structuredClone(initialFormValues);
			for (const [field, value] of Object.entries(previouslyApplied.fields)) {
				deepSet(
					effectiveInitialValues as Record<string, unknown>,
					field.split("."),
					value,
				);
			}
		}

		const nextValuesForHelper = {
			...form.values,
			model: knownModel.modelIdentifier,
		};
		const result = applyKnownModelDefaults({
			values: nextValuesForHelper,
			initialValues: effectiveInitialValues,
			provider: normalizedProvider,
			knownModel,
		});
		void form.setValues(result.values);
		// Selecting and blurring can both observe the same canonical model. This
		// ref skips the repeat apply so feedback does not flicker or duplicate.
		lastAppliedProviderModelRef.current = {
			provider: normalizedProvider,
			modelIdentifier: knownModel.modelIdentifier,
		};
		previouslyAppliedRef.current = {
			provider: normalizedProvider,
			modelIdentifier: knownModel.modelIdentifier,
			fields: Object.fromEntries(
				result.appliedFields.map((field) => [
					field,
					deepGet(result.values, field.split(".")),
				]),
			),
		};
		setFeedback(
			result.appliedFields.length > 0
				? `Defaults applied from ${knownModel.displayName}. Review and adjust before saving.`
				: null,
		);
	};

	const applyDefaultsOnExactCanonicalModel = () => {
		const found = findKnownModelByCanonicalId(normalizedProvider, currentModel);
		if (!found) {
			return;
		}

		const lastApplied = lastAppliedProviderModelRef.current;
		if (
			lastApplied?.provider === normalizedProvider &&
			lastApplied.modelIdentifier === found.modelIdentifier
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
			setSearchSnapshot("");
			clearAppliedModelState();
			return;
		}

		if (!option.knownModel) {
			void form.setFieldValue("model", option.model);
			clearAppliedModelState();
			return;
		}

		justSelectedRef.current = true;
		setSearchSnapshot(option.knownModel.modelIdentifier);
		applyDefaultsForKnownModel(option.knownModel);
	};

	const handleInputChange = (value: string) => {
		setSearchSnapshot(value);
		searchDirtyRef.current = true;
		setFeedback(null);
		lastAppliedProviderModelRef.current = null;
	};

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			setSearchSnapshot(currentModel);
			searchDirtyRef.current = false;
			closeIntentRef.current = null;
			setOpen(true);
			return;
		}

		setOpen(false);
		markTouched();

		const justSelected = justSelectedRef.current;
		const closeIntent = closeIntentRef.current;
		justSelectedRef.current = false;
		closeIntentRef.current = null;
		const typed = searchValueRef.current;

		if (closeIntent === "escape" || justSelected || !searchDirtyRef.current) {
			clearSearchSnapshot();
			return;
		}

		const aliasKnownModel = findKnownModelByAlias(normalizedProvider, typed);
		if (aliasKnownModel) {
			clearSearchSnapshot();
			return;
		}

		const exactKnownModel = findKnownModelByCanonicalId(
			normalizedProvider,
			typed,
		);
		if (exactKnownModel) {
			applyDefaultsForKnownModel(exactKnownModel);
			clearSearchSnapshot();
			return;
		}

		void form.setFieldValue("model", typed);
		previouslyAppliedRef.current = null;
		clearSearchSnapshot();
	};

	const handleBlur = (event: FocusEvent<HTMLDivElement>) => {
		if (open) {
			return;
		}

		const relatedTarget = event.relatedTarget;
		if (
			relatedTarget instanceof Node &&
			event.currentTarget.contains(relatedTarget)
		) {
			return;
		}
		markTouched();
		applyDefaultsOnExactCanonicalModel();
	};

	const handleKeyDownCapture = (event: KeyboardEvent<HTMLDivElement>) => {
		if (event.key === "Escape") {
			closeIntentRef.current = "escape";
		}
	};

	const renderControl = () => {
		if (!usesKnownModelCatalog) {
			return (
				<Input
					id={modelField.id}
					name={modelField.name}
					className={cn(
						"h-9 text-[13px] placeholder:text-content-disabled",
						hasError && "border-content-destructive",
					)}
					placeholder="e.g. gpt-5, claude-sonnet-4-5"
					value={modelField.value}
					onChange={modelField.onChange}
					onBlur={modelField.onBlur}
					disabled={disabled}
					aria-invalid={hasError}
					aria-describedby={errorId}
				/>
			);
		}

		return (
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
				inputValue={open ? searchValue : currentModel}
				onInputChange={handleInputChange}
				onEscapeKeyDown={() => {
					closeIntentRef.current = "escape";
				}}
				placeholder="e.g. gpt-5, claude-sonnet-4-5"
				noOptionsText="No matching known models. You can still use this identifier."
				className={cn(
					"h-9 text-[13px] placeholder:text-content-disabled",
					hasError && "border-content-destructive",
				)}
				triggerAriaInvalid={hasError}
				triggerAriaDescribedBy={errorId}
				disabled={disabled}
			/>
		);
	};

	return (
		<div
			className="grid gap-1.5"
			onBlur={usesKnownModelCatalog ? handleBlur : undefined}
			onKeyDownCapture={
				usesKnownModelCatalog ? handleKeyDownCapture : undefined
			}
		>
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
			{renderControl()}
			{hasError && (
				<p id={errorId} className="m-0 text-xs text-content-destructive">
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
