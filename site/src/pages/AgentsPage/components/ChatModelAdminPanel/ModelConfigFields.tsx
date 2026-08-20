import { type FormikContextType, getIn } from "formik";
import { InfoIcon } from "lucide-react";
import { type FC, Fragment, type ReactNode, useId } from "react";
import { useQuery } from "react-query";
import {
	type FieldSchema,
	getVisibleGeneralFields,
	getVisibleProviderFields,
	resolveProvider,
	snakeToCamel,
	toFormFieldKey,
} from "#/api/chatModelOptions";
import { aiModelPrices } from "#/api/queries/aiProviders";
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
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useDebouncedValue } from "#/hooks/debounce";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { cn } from "#/utils/cn";
import { microsToDollars } from "#/utils/currency";
import {
	findKnownModelByCanonicalId,
	findKnownModelByExactAlias,
	formatPricePerMillionTokens,
} from "./knownModels";
import {
	isFieldConflictDisabled,
	isVisibleWhenSatisfied,
	type ModelConfigFormBuildResult,
	type ModelFormValues,
} from "./modelConfigFormLogic";

const booleanFieldOptions = [
	{ label: "Off", value: "false" },
	{ label: "On", value: "true" },
	{ label: "Default", value: "" },
] as const;

/** Sentinel value for Select components to represent "no selection". */
const unsetSelectValue = "__unset__";

const isReasoningEffortField = (jsonName: string): boolean =>
	jsonName === "reasoning_effort.default" ||
	jsonName === "reasoning_effort.max";

// ── Helpers ────────────────────────────────────────────────────

/**
 * Suffix units displayed inside the input control. When present,
 * the field renders as an InputGroup with the suffix appended.
 */
const fieldSuffix: Record<string, string> = {
	max_output_tokens: "tokens",
	top_k: "tokens",
	"thinking.budget_tokens": "tokens",
	"thinking_config.thinking_budget": "tokens",
	max_completion_tokens: "tokens",
	"reasoning.max_tokens": "tokens",
	max_tool_calls: "calls",
};

/**
 * Placeholder overrides with range hints for numeric fields
 * where the valid range is more useful than an empty box.
 */
const placeholderOverrides: Record<string, string> = {
	temperature: "0.0 to 2.0",
	top_p: "0.0 to 1.0",
	presence_penalty: "-2.0 to 2.0",
	frequency_penalty: "-2.0 to 2.0",
};

function capitalize(s: string): string {
	return s.charAt(0).toUpperCase() + s.slice(1);
}

function snakeToPrettyLabel(field: FieldSchema): string {
	if (field.label) {
		return field.label;
	}
	const words = field.json_name.split(/[._]/);
	return words
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");
}

/**
 * Derive a sensible placeholder from the field schema type.
 */
function placeholderForField(field: FieldSchema): string {
	switch (field.type) {
		case "integer":
		case "number":
			return "";
		case "array":
			return "[]";
		case "object":
			return "{}";
		default:
			return "";
	}
}

// ── Generic field renderers ────────────────────────────────────

type FieldRenderContext = {
	form: FormikContextType<ModelFormValues>;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	disabled: boolean;
};

/** Label with an optional info tooltip for field descriptions. */
const FieldLabel: FC<{
	htmlFor: string;
	label: string;
	description?: string;
}> = ({ htmlFor, label, description }) => (
	<Label
		htmlFor={htmlFor}
		className="flex items-center gap-1 leading-6 text-content-primary"
	>
		{label}
		{description && (
			<Tooltip>
				<TooltipTrigger asChild>
					<InfoIcon className="size-3 text-content-secondary" />
				</TooltipTrigger>
				<TooltipContent side="top" className="max-w-[240px]">
					{description}
				</TooltipContent>
			</Tooltip>
		)}
	</Label>
);

const InputField: FC<
	FieldRenderContext & {
		fieldKey: string;
		errorKey?: string;
		label: string;
		description?: string;
		placeholder: string;
		suffix?: string;
	}
> = ({
	form,
	fieldErrors,
	disabled,
	fieldKey,
	errorKey,
	label,
	description,
	placeholder,
	suffix,
}) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[errorKey ?? fieldKey];
	const fieldProps = form.getFieldProps(fieldKey);

	const inputEl = suffix ? (
		<InputGroup className={cn(fieldError && "border-border-destructive")}>
			<InputGroupInput
				id={fieldKey}
				className="min-w-0 placeholder:text-content-disabled"
				placeholder={placeholder}
				{...fieldProps}
				disabled={disabled}
				aria-invalid={Boolean(fieldError)}
				aria-describedby={fieldError ? errorId : undefined}
			/>
			<InputGroupAddon align="inline-end">
				<span className="text-xs text-content-disabled">{suffix}</span>
			</InputGroupAddon>
		</InputGroup>
	) : (
		<Input
			id={fieldKey}
			className={cn(
				"min-w-0 placeholder:text-content-disabled",
				fieldError && "border-content-destructive",
			)}
			placeholder={placeholder}
			{...fieldProps}
			disabled={disabled}
			aria-invalid={Boolean(fieldError)}
			aria-describedby={fieldError ? errorId : undefined}
		/>
	);

	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<FieldLabel htmlFor={fieldKey} label={label} description={description} />
			{inputEl}
			{fieldError && (
				<p id={errorId} className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

const SelectField: FC<
	FieldRenderContext & {
		fieldKey: string;
		errorKey?: string;
		label: string;
		description?: string;
		options: readonly string[];
		placeholderLabel?: string;
	}
> = ({
	form,
	fieldErrors,
	disabled,
	fieldKey,
	errorKey,
	label,
	description,
	options,
	placeholderLabel = "Default",
}) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[errorKey ?? fieldKey];
	const currentValue = (getIn(form.values, fieldKey) as string) || "";
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<FieldLabel htmlFor={fieldKey} label={label} description={description} />
			<Select
				value={currentValue || unsetSelectValue}
				onValueChange={(value) =>
					void form.setFieldValue(
						fieldKey,
						value === unsetSelectValue ? "" : value,
					)
				}
				disabled={disabled}
			>
				<SelectTrigger
					id={fieldKey}
					className={cn(
						"min-w-0 shadow-none",
						fieldError && "border-content-destructive",
					)}
					aria-invalid={Boolean(fieldError)}
					aria-describedby={fieldError ? errorId : undefined}
				>
					<SelectValue placeholder={placeholderLabel} />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value={unsetSelectValue}>{placeholderLabel}</SelectItem>
					{options.map((option) => (
						<SelectItem key={option} value={option}>
							{capitalize(option)}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
			{fieldError && (
				<p id={errorId} className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

const SegmentedField: FC<
	FieldRenderContext & {
		fieldKey: string;
		errorKey?: string;
		label: string;
		description?: string;
		options: readonly { label: string; value: string }[];
	}
> = ({
	form,
	fieldErrors,
	disabled,
	fieldKey,
	errorKey,
	label,
	description,
	options,
}) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[errorKey ?? fieldKey];
	const currentValue = (getIn(form.values, fieldKey) as string) || "";

	return (
		<div className="flex min-w-0 flex-col gap-1.5 self-stretch">
			<div className="flex items-center gap-1 text-sm font-normal leading-6 text-content-primary">
				<span>{label}</span>
				{description && (
					<Tooltip>
						<TooltipTrigger asChild>
							<InfoIcon className="size-3 text-content-secondary" />
						</TooltipTrigger>
						<TooltipContent side="top" className="max-w-[240px]">
							{description}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			<div
				role="radiogroup"
				aria-label={label}
				className={cn(
					"flex w-full items-center gap-0.75 rounded-lg border border-solid border-border p-2",
					fieldError && "border-content-destructive",
				)}
			>
				{options.map((opt) => {
					const isActive = currentValue === opt.value;
					return (
						<button
							key={opt.value}
							type="button"
							role="radio"
							aria-checked={isActive}
							disabled={disabled}
							className={cn(
								"flex h-6 flex-1 cursor-pointer items-center justify-center gap-2.5 rounded-xl border-0 px-2 pb-px text-sm font-normal leading-6 transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
								isActive
									? "rounded bg-surface-tertiary text-content-primary"
									: "bg-transparent text-content-secondary hover:text-content-primary",
								disabled && "pointer-events-none opacity-60",
							)}
							onClick={() => void form.setFieldValue(fieldKey, opt.value)}
						>
							{opt.label}
						</button>
					);
				})}
			</div>
			{fieldError && (
				<p id={errorId} className="m-0 w-full text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

const JSONField: FC<
	FieldRenderContext & {
		fieldKey: string;
		errorKey?: string;
		label: string;
		description?: string;
		placeholder: string;
	}
> = ({
	form,
	fieldErrors,
	disabled,
	fieldKey,
	errorKey,
	label,
	description,
	placeholder,
}) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[errorKey ?? fieldKey];
	const fieldProps = form.getFieldProps(fieldKey);
	// Only surface the error once the field has been blurred, so a partially
	// typed array like "[" doesn't complain mid-edit.
	const showError =
		Boolean(fieldError) && Boolean(getIn(form.touched, fieldKey));
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<FieldLabel htmlFor={fieldKey} label={label} description={description} />
			<Textarea
				id={fieldKey}
				rows={1}
				className={cn(
					"min-h-0 resize-y font-mono text-xs leading-tight placeholder:text-content-disabled",
					showError && "border-content-destructive",
				)}
				placeholder={placeholder}
				{...fieldProps}
				disabled={disabled}
				aria-invalid={showError}
				aria-describedby={showError ? errorId : undefined}
			/>
			{showError && (
				<p id={errorId} className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

// ── Schema-driven field renderer ───────────────────────────────

interface SchemaFieldProps extends FieldRenderContext {
	field: FieldSchema;
	fieldKey: string;
	errorKey: string;
}

/**
 * Render a single field from the schema using the appropriate
 * generic renderer based on its `input_type`.
 */
const SchemaField: FC<SchemaFieldProps> = ({
	field,
	fieldKey,
	errorKey,
	form,
	fieldErrors,
	disabled,
}) => {
	const label = snakeToPrettyLabel(field);
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };

	switch (field.input_type) {
		case "input":
			return (
				<InputField
					{...ctx}
					fieldKey={fieldKey}
					errorKey={errorKey}
					label={label}
					description={field.description}
					placeholder={
						placeholderOverrides[field.json_name] ?? placeholderForField(field)
					}
					suffix={fieldSuffix[field.json_name]}
				/>
			);
		case "select": {
			// Booleans keep the on/off/default segmented switch; every string
			// enum renders as a dropdown so the switch stays a tri-state control.
			if (field.type === "boolean") {
				return (
					<SegmentedField
						{...ctx}
						fieldKey={fieldKey}
						errorKey={errorKey}
						label={label}
						description={field.description}
						options={booleanFieldOptions}
					/>
				);
			}
			const options: readonly string[] = field.enum ?? [];
			return (
				<SelectField
					{...ctx}
					fieldKey={fieldKey}
					errorKey={errorKey}
					label={label}
					description={field.description}
					options={options}
				/>
			);
		}
		case "json":
			return (
				<JSONField
					{...ctx}
					fieldKey={fieldKey}
					errorKey={errorKey}
					label={label}
					description={field.description}
					placeholder={placeholderForField(field)}
				/>
			);
		default:
			return null;
	}
};

// ── Main component ─────────────────────────────────────────────

/**
 * How many grid columns a field should span in the 3-col layout.
 *   1 = default (inputs, selects, boolean switches)
 *   3 = full-width (json textareas, which need room for multi-line content)
 */
function colSpan(field: FieldSchema): 1 | 3 {
	if (field.input_type === "json") {
		return 3;
	}
	return 1;
}

const colSpanClass: Record<1 | 3, string | undefined> = {
	1: undefined,
	3: "sm:col-span-full",
};

interface ModelConfigFieldsProps {
	provider: string;
	form: FormikContextType<ModelFormValues>;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	disabled: boolean;
	children?: ReactNode;
}

/**
 * Provider-specific fields (reasoning, tool calls, etc.) that
 * should be visible at the top level of the model form.
 *
 * Fields and their input types are driven by the auto-generated
 * schema in `api/chatModelOptions`.
 */
export const ModelConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	disabled,
	children,
}) => {
	const normalized = normalizeProvider(provider);
	const resolved = resolveProvider(normalized);
	const fields = getVisibleProviderFields(normalized);

	if (fields.length === 0) {
		return null;
	}

	const fieldValueByName = (jsonName: string): unknown =>
		getIn(form.values, `config.${toFormFieldKey(resolved, jsonName)}`);

	const isFieldVisible = (field: FieldSchema): boolean =>
		isVisibleWhenSatisfied(field, fieldValueByName);

	// Sort wider fields to the end so compact fields fill the
	// grid first, keeping the layout dense.
	const sorted = [...fields]
		.filter(isFieldVisible)
		.sort((a, b) => colSpan(a) - colSpan(b));

	return (
		<div className="grid min-w-0 gap-3 sm:grid-cols-3">
			{sorted.map((field) => {
				const fieldKey = `config.${toFormFieldKey(resolved, field.json_name)}`;
				const errorKey = toFormFieldKey(resolved, field.json_name);
				return (
					<Fragment key={fieldKey}>
						<div className={colSpanClass[colSpan(field)]}>
							<SchemaField
								field={field}
								fieldKey={fieldKey}
								errorKey={errorKey}
								form={form}
								fieldErrors={fieldErrors}
								disabled={
									disabled || isFieldConflictDisabled(field, fieldValueByName)
								}
							/>
						</div>
						{field.json_name === "thinking.budget_tokens" && children}
					</Fragment>
				);
			})}
			{!sorted.some((field) => field.json_name === "thinking.budget_tokens") &&
				children}
		</div>
	);
};

/** Reasoning effort selects, outside Advanced. */
export const ReasoningEffortConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	disabled,
}) => {
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };
	const fields = getVisibleGeneralFields(provider)
		.filter(({ json_name }) => isReasoningEffortField(json_name))
		.reverse();

	return (
		<>
			{fields.map((field) => {
				const camelName = field.json_name
					.split(".")
					.map(snakeToCamel)
					.join(".");
				const fieldKey = `config.${camelName}`;

				return (
					<SelectField
						key={fieldKey}
						{...ctx}
						fieldKey={fieldKey}
						errorKey={camelName}
						label={snakeToPrettyLabel(field)}
						description={field.description}
						options={field.enum ?? []}
						placeholderLabel="Not set"
					/>
				);
			})}
		</>
	);
};

/** See ReasoningEffortConfigFields for reasoning effort fields. */
export const GeneralModelConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	disabled,
}) => {
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };
	const fields = getVisibleGeneralFields(provider).filter(
		({ json_name }) => !isReasoningEffortField(json_name),
	);

	return (
		<>
			{fields.map((field) => {
				const camelName = field.json_name
					.split(".")
					.map(snakeToCamel)
					.join(".");
				const fieldKey = `config.${camelName}`;

				return (
					<div
						key={fieldKey}
						className={cn("min-w-0", colSpanClass[colSpan(field)])}
					>
						<SchemaField
							{...ctx}
							field={field}
							fieldKey={fieldKey}
							errorKey={camelName}
						/>
					</div>
				);
			})}
		</>
	);
};

type ModelCosts = {
	inputCost?: number;
	outputCost?: number;
	cacheReadCost?: number;
	cacheWriteCost?: number;
};

const priceEstimateFields: ReadonlyArray<[string, keyof ModelCosts]> = [
	["Input", "inputCost"],
	["Output", "outputCost"],
	["Cache read", "cacheReadCost"],
	["Cache write", "cacheWriteCost"],
];

const priceOrUndefined = (micros: number | null): number | undefined =>
	micros === null ? undefined : microsToDollars(micros);

export const PricingEstimateFields: FC<{
	provider: string;
	model: string;
}> = ({ provider, model }) => {
	const fieldIdPrefix = useId();
	const aibridgeEntitled = Boolean(useFeatureVisibility().aibridge);
	const normalizedProvider = normalizeProvider(provider);
	const trimmedModel = model.trim();
	// Debounce the pair so the lookup never mixes a new provider with the
	// previous model identifier.
	const debouncedLookup = useDebouncedValue(
		{ provider: normalizedProvider, model: trimmedModel },
		500,
	);
	const entitledWithProvider = aibridgeEntitled && normalizedProvider !== "";
	const livePricesQuery = useQuery({
		...aiModelPrices(debouncedLookup.provider, debouncedLookup.model),
		enabled: entitledWithProvider && debouncedLookup.model !== "",
	});
	const debouncePending =
		entitledWithProvider &&
		(debouncedLookup.provider !== normalizedProvider ||
			debouncedLookup.model !== trimmedModel);
	const livePriceLoading =
		debouncePending ||
		(livePricesQuery.fetchStatus !== "idle" && !livePricesQuery.isSuccess);
	const livePrice = debouncePending ? undefined : livePricesQuery.data?.[0];

	const knownModel =
		findKnownModelByCanonicalId(normalizedProvider, trimmedModel) ??
		findKnownModelByExactAlias(normalizedProvider, trimmedModel);

	// A price book row is the deployment's own pricing for the model, so it
	// wins outright. A null field on that row means the category is unpriced
	// (the cost engine bills it as zero), not that the catalog should fill it
	// in. The catalog is only the fallback when the model has no row at all.
	const costs: ModelCosts | undefined = livePrice
		? {
				inputCost: priceOrUndefined(livePrice.input_price),
				outputCost: priceOrUndefined(livePrice.output_price),
				cacheReadCost: priceOrUndefined(livePrice.cache_read_price),
				cacheWriteCost: priceOrUndefined(livePrice.cache_write_price),
			}
		: knownModel;

	if (!debouncePending && livePricesQuery.isError) {
		return (
			<p className="m-0 flex items-center gap-1.5 text-xs text-content-secondary sm:col-span-full">
				<InfoIcon className="size-3.5 shrink-0" />
				Couldn't load pricing.
			</p>
		);
	}

	if (
		!livePriceLoading &&
		(costs === undefined ||
			priceEstimateFields.every(([, key]) => costs[key] === undefined))
	) {
		return (
			<p className="m-0 text-xs text-content-secondary sm:col-span-full">
				No pricing data for this model.
			</p>
		);
	}

	return (
		<>
			{priceEstimateFields.map(([label, key]) => {
				const cost = costs?.[key];
				const fieldId = `${fieldIdPrefix}-${label.toLowerCase().replace(/\s+/g, "-")}`;
				const price =
					cost === undefined ? undefined : formatPricePerMillionTokens(cost);
				return (
					<div key={label} className="flex min-w-0 flex-col gap-1.5">
						<FieldLabel htmlFor={fieldId} label={label} />
						<InputGroup className="cursor-not-allowed bg-surface-secondary">
							<InputGroupAddon align="inline-start">
								{price?.belowThreshold ? "<$" : "$"}
							</InputGroupAddon>
							{livePriceLoading ? (
								<Skeleton
									aria-label={`${label} price loading`}
									className="mx-3 h-2 w-2/5 flex-1 rounded-full"
								/>
							) : (
								<InputGroupInput
									id={fieldId}
									className="min-w-0 cursor-not-allowed text-content-secondary"
									value={price?.value ?? ""}
									aria-describedby={
										price?.belowThreshold ? `${fieldId}-threshold` : undefined
									}
									readOnly
								/>
							)}
							{price?.belowThreshold && !livePriceLoading && (
								<span id={`${fieldId}-threshold`} className="sr-only">
									{`less than $${price.value} USD per million tokens`}
								</span>
							)}
							<InputGroupAddon align="inline-end">
								<span className="text-xs text-content-disabled">
									USD/1M tokens
								</span>
							</InputGroupAddon>
						</InputGroup>
					</div>
				);
			})}
		</>
	);
};
