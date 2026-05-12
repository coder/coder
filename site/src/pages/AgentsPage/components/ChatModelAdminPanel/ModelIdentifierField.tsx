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
	findKnownModelByCanonicalId,
	findKnownModelByExactAlias,
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
	// Mirror of `open` for synchronous reads from blur handlers; React state may not have committed when Radix shifts focus on open.
	const openRef = useRef(false);
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
	const knownModelOptions = searchKnownModels(
		normalizedProvider,
		activeSearchQuery,
	).map(knownModelToOption);
	const selectedKnownModel = findKnownModelByCanonicalId(
		normalizedProvider,
		currentModel,
	);
	const selectedOption: ModelIdentifierOption | null = (() => {
		if (selectedKnownModel) return knownModelToOption(selectedKnownModel);
		if (currentModel) return { model: currentModel, displayName: currentModel };
		return null;
	})();
	const hasError = Boolean(modelField.error);
	const errorId = hasError ? `${modelField.id}-error` : undefined;

	// biome-ignore lint/correctness/useExhaustiveDependencies: Provider reset.
	useEffect(() => {
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

	const clearAppliedModelFeedback = () => {
		setFeedback(null);
		lastAppliedProviderModelRef.current = null;
	};

	const applyDefaultsForKnownModel = (knownModel: KnownModel) => {
		if (knownModel.provider !== normalizedProvider) {
			return;
		}

		const nextValuesForHelper = {
			...form.values,
			model: knownModel.modelIdentifier,
		};
		let effectiveInitialValues = initialFormValues;
		const previouslyApplied = previouslyAppliedRef.current;
		const previouslyAppliedFields: Record<string, unknown> =
			previouslyApplied?.provider === normalizedProvider
				? { ...previouslyApplied.fields }
				: {};
		if (previouslyApplied?.provider === normalizedProvider) {
			effectiveInitialValues = structuredClone(initialFormValues);
			// This map persists for the form session and stores the last value
			// written by Known Model defaulting for each path. A path is safe
			// to overwrite only when the current value still matches either
			// the original initial value or this stored Known Model value.
			for (const [field, value] of Object.entries(previouslyApplied.fields)) {
				const segments = field.split(".");
				if (deepGet(nextValuesForHelper, segments) !== value) {
					continue;
				}
				deepSet(
					effectiveInitialValues as Record<string, unknown>,
					segments,
					value,
				);
			}
		}

		const result = applyKnownModelDefaults({
			values: nextValuesForHelper,
			initialValues: effectiveInitialValues,
			provider: normalizedProvider,
			knownModel,
		});
		const appliedFields = new Set(result.appliedFields);
		for (const [field, value] of Object.entries(previouslyAppliedFields)) {
			if (appliedFields.has(field)) {
				continue;
			}

			const segments = field.split(".");
			if (deepGet(result.values, segments) !== value) {
				continue;
			}

			const initialValue = deepGet(initialFormValues, segments);
			deepSet(result.values as Record<string, unknown>, segments, initialValue);
			previouslyAppliedFields[field] = initialValue;
		}
		for (const field of result.appliedFields) {
			previouslyAppliedFields[field] = deepGet(result.values, field.split("."));
		}
		void form.setValues(result.values);
		// Selecting and blurring can both observe the same canonical model. This
		// ref skips the repeat apply so feedback does not flicker or duplicate.
		lastAppliedProviderModelRef.current = {
			provider: normalizedProvider,
			modelIdentifier: knownModel.modelIdentifier,
		};
		previouslyAppliedRef.current = {
			provider: normalizedProvider,
			fields: previouslyAppliedFields,
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
			clearAppliedModelFeedback();
			return;
		}

		if (!option.knownModel) {
			void form.setFieldValue("model", option.model);
			clearAppliedModelFeedback();
			return;
		}

		justSelectedRef.current = true;
		setSearchSnapshot(option.knownModel.modelIdentifier);
		applyDefaultsForKnownModel(option.knownModel);
	};

	const handleInputChange = (value: string) => {
		setSearchSnapshot(value);
		searchDirtyRef.current = true;
	};

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			openRef.current = true;
			setSearchSnapshot(currentModel);
			searchDirtyRef.current = false;
			closeIntentRef.current = null;
			setOpen(true);
			return;
		}

		openRef.current = false;
		setOpen(false);

		const justSelected = justSelectedRef.current;
		const closeIntent = closeIntentRef.current;
		justSelectedRef.current = false;
		closeIntentRef.current = null;
		const typed = searchValueRef.current;

		// A selection already wrote the canonical model via handleSelect; mark
		// touched so validation reflects the committed value.
		if (justSelected) {
			markTouched();
			clearSearchSnapshot();
			return;
		}

		// Escape and open-then-close-without-typing must leave validation
		// untouched. Marking touched here would surface "Model ID is required."
		// for an admin who clicked the field, changed their mind, and clicked
		// off without ever attempting to commit a value.
		if (closeIntent === "escape" || !searchDirtyRef.current) {
			clearSearchSnapshot();
			return;
		}

		// All remaining paths commit a value, so mark touched to surface validation.
		markTouched();

		const exactKnownModel = findKnownModelByCanonicalId(
			normalizedProvider,
			typed,
		);
		if (exactKnownModel) {
			const lastApplied = lastAppliedProviderModelRef.current;
			const alreadyApplied =
				lastApplied?.provider === normalizedProvider &&
				lastApplied.modelIdentifier === exactKnownModel.modelIdentifier;
			if (!alreadyApplied) {
				applyDefaultsForKnownModel(exactKnownModel);
			}
			clearSearchSnapshot();
			return;
		}

		const aliasKnownModel = findKnownModelByExactAlias(
			normalizedProvider,
			typed,
		);
		if (aliasKnownModel) {
			clearSearchSnapshot();
			return;
		}

		void form.setFieldValue("model", typed);
		clearAppliedModelFeedback();
		clearSearchSnapshot();
	};

	const handleBlur = (event: FocusEvent<HTMLDivElement>) => {
		const relatedTarget = event.relatedTarget;
		if (
			relatedTarget instanceof Node &&
			event.currentTarget.contains(relatedTarget)
		) {
			return;
		}
		// Popover is rendered, so let Radix handle the close.
		if (openRef.current && knownModelOptions.length > 0) {
			return;
		}
		// Popover is auto-hidden with no matches, but isOpen is still true.
		// Treat blur as close intent so close logic can commit or restore.
		if (openRef.current) {
			handleOpenChange(false);
			return;
		}
		// Only mark touched once a value is in play. An empty currentModel
		// here means the user left the wrapper without committing anything,
		// so leave validation untouched (Formik flips touched on submit).
		if (currentModel !== "") {
			markTouched();
		}
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

		// Required field; off-catalog typing is the supported clear or replace path.
		return (
			<Autocomplete
				id={modelField.id}
				clearable={false}
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
				inputValue={activeSearchQuery}
				onInputChange={handleInputChange}
				onEscapeKeyDown={() => {
					closeIntentRef.current = "escape";
				}}
				inlineSearch
				onEnterEmpty={() => handleOpenChange(false)}
				placeholder="e.g. gpt-5, claude-sonnet-4-5"
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
