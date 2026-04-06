import { type FormikContextType, getIn } from "formik";
import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import {
	type FieldSchema,
	getVisibleGeneralFields,
	getVisibleProviderFields,
	resolveProvider,
	snakeToCamel,
	toFormFieldKey,
} from "#/api/chatModelOptions";
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
import { Textarea } from "#/components/Textarea/Textarea";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { normalizeProvider } from "./helpers";
import type {
	ModelConfigFormBuildResult,
	ModelFormValues,
} from "./modelConfigFormLogic";
import {
	getPricingPlaceholderForField,
	pricingFieldNames,
} from "./pricingFields";

/** Sentinel value for Select components to represent "no selection". */
const unsetSelectValue = "__unset__";

// ── Helpers ────────────────────────────────────────────────────

/** Short display labels for pricing fields to avoid overly verbose names. */
const shortLabelOverrides: Record<string, string> = {
	"cost.input_price_per_million_tokens": "Input",
	"cost.output_price_per_million_tokens": "Output",
	"cost.cache_read_price_per_million_tokens": "Cache Read",
	"cost.cache_write_price_per_million_tokens": "Cache Write",
};

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
	temperature: "0.0–2.0",
	top_p: "0.0–1.0",
	presence_penalty: "-2.0–2.0",
	frequency_penalty: "-2.0–2.0",
};

/**
 * Convert a dot-and-underscore-separated json_name into a
 * human-readable label. Uses short overrides for pricing fields
 * when available.
 *
 * @example
 * snakeToPrettyLabel("thinking.budget_tokens") // "Thinking Budget Tokens"
 * snakeToPrettyLabel("reasoning_effort")        // "Reasoning Effort"
 */
/** Capitalize the first letter of a string. */
function capitalize(s: string): string {
	return s.charAt(0).toUpperCase() + s.slice(1);
}

function snakeToPrettyLabel(field: FieldSchema): string {
	if (field.label) {
		return field.label;
	}
	if (shortLabelOverrides[field.json_name]) {
		return shortLabelOverrides[field.json_name];
	}
	return field.json_name
		.split(/[._]/)
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");
}

/**
 * Derive a sensible placeholder from the field schema type.
 */
function placeholderForField(field: FieldSchema): string {
	const pricingPlaceholder = getPricingPlaceholderForField(field.json_name);
	if (pricingPlaceholder !== undefined) {
		return pricingPlaceholder;
	}

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
		className="inline-flex items-center gap-1 text-[13px] font-medium text-content-primary"
	>
		{label}
		{description && (
			<Tooltip>
				<TooltipTrigger asChild>
					<InfoIcon className="h-3 w-3 text-content-secondary" />
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
		<InputGroup
			className={cn("h-9", fieldError && "border-border-destructive")}
		>
			<InputGroupInput
				id={fieldKey}
				className="h-9 min-w-0 text-[13px] placeholder:text-content-disabled"
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
				"h-9 min-w-0 text-[13px] placeholder:text-content-disabled",
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
						"h-9 min-w-0 text-[13px]",
						fieldError && "border-content-destructive",
					)}
					aria-invalid={Boolean(fieldError)}
					aria-describedby={fieldError ? errorId : undefined}
				>
					<SelectValue placeholder="Default" />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value={unsetSelectValue}>Default</SelectItem>
					{options.map((option) => (
						<SelectItem key={option} value={option}>
							{option}
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
		<div className="flex min-w-0 flex-col gap-1.5">
			<FieldLabel htmlFor={fieldKey} label={label} description={description} />
			<div
				role="radiogroup"
				aria-label={label}
				className={cn(
					"flex h-9 items-stretch rounded-md border border-solid border-border p-0.5",
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
								"h-8 flex-1 cursor-pointer rounded-[5px] border-0 px-3 text-[13px] font-medium transition-colors",
								isActive
									? "bg-surface-secondary text-content-primary"
									: "bg-transparent text-content-secondary hover:text-content-primary",
								disabled && "pointer-events-none opacity-60",
							)}
							onClick={() =>
								void form.setFieldValue(fieldKey, isActive ? "" : opt.value)
							}
						>
							{opt.label}
						</button>
					);
				})}
			</div>
			{fieldError && (
				<p id={errorId} className="m-0 text-xs text-content-destructive">
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
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<FieldLabel htmlFor={fieldKey} label={label} description={description} />
			<Textarea
				id={fieldKey}
				rows={1}
				className={cn(
					"min-h-0 resize-y font-mono text-xs leading-tight placeholder:text-content-disabled",
					fieldError && "border-content-destructive",
				)}
				placeholder={placeholder}
				{...fieldProps}
				disabled={disabled}
				aria-invalid={Boolean(fieldError)}
				aria-describedby={fieldError ? errorId : undefined}
			/>
			{fieldError && (
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
			if (field.type === "boolean") {
				return (
					<SegmentedField
						{...ctx}
						fieldKey={fieldKey}
						errorKey={errorKey}
						label={label}
						description={field.description}
						options={[
							{ label: "On", value: "true" },
							{ label: "Off", value: "false" },
						]}
					/>
				);
			}
			const options: readonly string[] = field.enum ?? [];
			const maxSegmented = 6;
			if (options.length > 0 && options.length <= maxSegmented) {
				return (
					<SegmentedField
						{...ctx}
						fieldKey={fieldKey}
						errorKey={errorKey}
						label={label}
						description={field.description}
						options={options.map((v) => ({ label: capitalize(v), value: v }))}
					/>
				);
			}
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
 *   1 = default (inputs, booleans, small enums ≤3)
 *   3 = full-width (4+ option enums, json textareas)
 */
function colSpan(field: FieldSchema): 1 | 3 {
	if (field.input_type === "json") {
		return 3;
	}
	if (
		field.input_type === "select" &&
		field.type !== "boolean" &&
		(field.enum?.length ?? 0) > 3
	) {
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
}) => {
	const normalized = normalizeProvider(provider);
	const resolved = resolveProvider(normalized);
	const fields = getVisibleProviderFields(normalized);

	if (fields.length === 0) {
		return null;
	}

	const ctx: FieldRenderContext = { form, fieldErrors, disabled };

	// Sort wider fields to the end so compact fields fill the
	// grid first, keeping the layout dense.
	const sorted = [...fields].sort((a, b) => colSpan(a) - colSpan(b));

	return (
		<div className="grid min-w-0 gap-3 sm:grid-cols-3">
			{sorted.map((field) => {
				const fieldKey = `config.${toFormFieldKey(resolved, field.json_name)}`;
				const errorKey = toFormFieldKey(resolved, field.json_name);
				return (
					<div key={fieldKey} className={colSpanClass[colSpan(field)]}>
						<SchemaField
							field={field}
							fieldKey={fieldKey}
							errorKey={errorKey}
							{...ctx}
						/>
					</div>
				);
			})}
		</div>
	);
};

/**
 * Shared renderer for general model config fields backed by the
 * top-level ChatModelCallConfig schema.
 */
const GeneralFieldsGroup: FC<
	ModelConfigFieldsProps & {
		fields: FieldSchema[];
		suppressDescriptions?: boolean;
	}
> = ({ form, fieldErrors, disabled, fields, suppressDescriptions }) => {
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };

	return (
		<>
			{fields.map((field) => {
				// General field keys support nested json_name values, such as
				// cost.input_price_per_million_tokens.
				const camelName = field.json_name
					.split(".")
					.map(snakeToCamel)
					.join(".");
				const fieldKey = `config.${camelName}`;
				const label = snakeToPrettyLabel(field);

				return (
					<InputField
						key={fieldKey}
						{...ctx}
						fieldKey={fieldKey}
						errorKey={camelName}
						label={label}
						description={suppressDescriptions ? undefined : field.description}
						placeholder={
							placeholderOverrides[field.json_name] ??
							placeholderForField(field)
						}
						suffix={fieldSuffix[field.json_name]}
					/>
				);
			})}
		</>
	);
};

/**
 * Pricing fields rendered with $ prefix and /1M suffix using
 * InputGroup for a compact, readable layout.
 */
export const PricingModelConfigFields: FC<ModelConfigFieldsProps> = ({
	form,
	fieldErrors,
	disabled,
}) => {
	const fields = getVisibleGeneralFields().filter(({ json_name }) =>
		pricingFieldNames.has(json_name),
	);

	return (
		<>
			{fields.map((field) => {
				const camelName = field.json_name
					.split(".")
					.map(snakeToCamel)
					.join(".");
				const fieldKey = `config.${camelName}`;
				const label = snakeToPrettyLabel(field);
				const errorId = `${fieldKey}-error`;
				const fieldError = fieldErrors[camelName];
				const fieldProps = form.getFieldProps(fieldKey);

				return (
					<div key={fieldKey} className="flex min-w-0 flex-col gap-1.5">
						<FieldLabel htmlFor={fieldKey} label={label} />
						<InputGroup
							className={cn("h-9", fieldError && "border-border-destructive")}
						>
							<InputGroupAddon align="inline-start">$</InputGroupAddon>
							<InputGroupInput
								id={fieldKey}
								className="h-9 min-w-0 text-[13px] placeholder:text-content-disabled"
								placeholder="0"
								{...fieldProps}
								disabled={disabled}
								aria-invalid={Boolean(fieldError)}
								aria-describedby={fieldError ? errorId : undefined}
							/>
							<InputGroupAddon align="inline-end">
								<span className="text-xs text-content-disabled">
									USD/1M tokens
								</span>
							</InputGroupAddon>
						</InputGroup>
						{fieldError && (
							<p id={errorId} className="m-0 text-xs text-content-destructive">
								{fieldError}
							</p>
						)}
					</div>
				);
			})}
		</>
	);
};

/**
 * General model config fields (max output tokens, temperature,
 * top P, etc.) intended to be shown under an "Advanced" section.
 *
 * Fields are driven by the auto-generated schema in
 * `api/chatModelOptions`.
 */
export const GeneralModelConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	disabled,
}) => {
	return (
		<GeneralFieldsGroup
			provider={provider}
			form={form}
			fieldErrors={fieldErrors}
			disabled={disabled}
			fields={getVisibleGeneralFields().filter(
				({ json_name }) => !pricingFieldNames.has(json_name),
			)}
		/>
	);
};
