import {
	type FieldSchema,
	getVisibleGeneralFields,
	getVisibleProviderFields,
	resolveProvider,
	snakeToCamel,
	toFormFieldKey,
} from "api/chatModelOptions";
import { type FormikContextType, getIn } from "formik";
import type { FC } from "react";
import { cn } from "utils/cn";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Textarea } from "#/components/Textarea/Textarea";
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

/**
 * Convert a dot-and-underscore-separated json_name into a
 * human-readable label.
 *
 * @example
 * snakeToPrettyLabel("thinking.budget_tokens") // "Thinking Budget Tokens"
 * snakeToPrettyLabel("reasoning_effort")        // "Reasoning Effort"
 */
function snakeToPrettyLabel(jsonName: string): string {
	return jsonName
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

const InputField: FC<
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
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
			{description && (
				<p className="m-0 text-xs text-content-secondary">{description}</p>
			)}
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
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
			{description && (
				<p className="m-0 text-xs text-content-secondary">{description}</p>
			)}
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
					<SelectValue placeholder="Unset" />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value={unsetSelectValue}>Unset</SelectItem>
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
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
			{description && (
				<p className="m-0 text-xs text-content-secondary">{description}</p>
			)}
			<Textarea
				id={fieldKey}
				className={cn(
					"min-h-[96px] font-mono text-xs placeholder:text-content-disabled",
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
	const label = snakeToPrettyLabel(field.json_name);
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
					placeholder={placeholderForField(field)}
				/>
			);
		case "select": {
			const options: readonly string[] =
				field.enum ?? (field.type === "boolean" ? ["true", "false"] : []);
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

	return (
		<div className="grid min-w-0 gap-3 sm:grid-cols-2">
			{fields.map((field) => {
				const fieldKey = `config.${toFormFieldKey(resolved, field.json_name)}`;
				const errorKey = toFormFieldKey(resolved, field.json_name);
				return (
					<SchemaField
						key={fieldKey}
						field={field}
						fieldKey={fieldKey}
						errorKey={errorKey}
						{...ctx}
					/>
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
	}
> = ({ form, fieldErrors, disabled, fields }) => {
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
				const label = snakeToPrettyLabel(field.json_name);

				return (
					<InputField
						key={fieldKey}
						{...ctx}
						fieldKey={fieldKey}
						errorKey={camelName}
						label={label}
						description={field.description}
						placeholder={placeholderForField(field)}
					/>
				);
			})}
		</>
	);
};

/**
 * General pricing fields shown in the main form body so admins can
 * define optional pricing metadata without opening the advanced section.
 */
export const PricingModelConfigFields: FC<ModelConfigFieldsProps> = ({
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
			fields={getVisibleGeneralFields().filter(({ json_name }) =>
				pricingFieldNames.has(json_name),
			)}
		/>
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
