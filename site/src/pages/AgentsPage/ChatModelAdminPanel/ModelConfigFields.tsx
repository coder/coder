import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Textarea } from "components/Textarea/Textarea";
import { type FormikContextType, getIn } from "formik";
import type { FC } from "react";
import { cn } from "utils/cn";
import { normalizeProvider } from "./helpers";
import type {
	ModelConfigFormBuildResult,
	ModelFormValues,
} from "./modelConfigFormLogic";

export const modelConfigReasoningEffortOptions = [
	"minimal",
	"low",
	"medium",
	"high",
	"xhigh",
	"none",
] as const;

export const modelConfigAnthropicEffortOptions = [
	"low",
	"medium",
	"high",
	"max",
] as const;

export const modelConfigTextVerbosityOptions = [
	"low",
	"medium",
	"high",
] as const;

/** Sentinel value for Select components to represent "no selection". */
const unsetSelectValue = "__unset__";

// ── Generic field renderers ────────────────────────────────────

type FieldRenderContext = {
	form: FormikContextType<ModelFormValues>;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	disabled: boolean;
};

const InputField: FC<
	FieldRenderContext & {
		fieldKey: string;
		label: string;
		placeholder: string;
	}
> = ({ form, fieldErrors, disabled, fieldKey, label, placeholder }) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[fieldKey];
	const fieldProps = form.getFieldProps(fieldKey);
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
			<Input
				id={fieldKey}
				className={cn(
					"h-9 min-w-0 text-[13px] placeholder:text-content-disabled",
					fieldError && "border-content-destructive",
				)}
				placeholder={placeholder}
				{...fieldProps}
				disabled={disabled}
				aria-invalid={!!fieldError}
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
		label: string;
		options: readonly string[];
	}
> = ({ form, fieldErrors, disabled, fieldKey, label, options }) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[fieldKey];
	const currentValue = (getIn(form.values, fieldKey) as string) || "";
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
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
					aria-invalid={!!fieldError}
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
		label: string;
		placeholder: string;
	}
> = ({ form, fieldErrors, disabled, fieldKey, label, placeholder }) => {
	const errorId = `${fieldKey}-error`;
	const fieldError = fieldErrors[fieldKey];
	const fieldProps = form.getFieldProps(fieldKey);
	return (
		<div className="flex min-w-0 flex-col gap-1.5">
			<Label
				htmlFor={fieldKey}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}
			</Label>
			<Textarea
				id={fieldKey}
				className={cn(
					"min-h-[96px] font-mono text-xs placeholder:text-content-disabled",
					fieldError && "border-content-destructive",
				)}
				placeholder={placeholder}
				{...fieldProps}
				disabled={disabled}
				aria-invalid={!!fieldError}
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

// ── Provider-specific field sets ───────────────────────────────

const OpenAIFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="config.openai.reasoningEffort"
				label="Reasoning Effort"
				options={modelConfigReasoningEffortOptions}
			/>
			<SelectField
				{...props}
				fieldKey="config.openai.parallelToolCalls"
				label="Parallel Tool Calls"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="config.openai.textVerbosity"
				label="Text Verbosity"
				options={modelConfigTextVerbosityOptions}
			/>
			<InputField
				{...props}
				fieldKey="config.openai.serviceTier"
				label="Service Tier"
				placeholder="auto"
			/>
			<InputField
				{...props}
				fieldKey="config.openai.reasoningSummary"
				label="Reasoning Summary"
				placeholder="detailed"
			/>
			<InputField
				{...props}
				fieldKey="config.openai.user"
				label="User"
				placeholder="end-user-id"
			/>
	</div>
);

const AnthropicFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="config.anthropic.effort"
				label="Output Effort"
				options={modelConfigAnthropicEffortOptions}
			/>
			<InputField
				{...props}
				fieldKey="config.anthropic.thinkingBudgetTokens"
				label="Thinking Budget Tokens"
				placeholder="4000"
			/>
			<SelectField
				{...props}
				fieldKey="config.anthropic.sendReasoning"
				label="Send Reasoning"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="config.anthropic.disableParallelToolUse"
				label="Disable Parallel Tool Use"
				options={["true", "false"]}
			/>
	</div>
);

const GoogleFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
		<InputField
			{...props}
			fieldKey="config.google.thinkingBudget"
			label="Thinking Budget"
			placeholder="1024"
		/>
		<SelectField
			{...props}
			fieldKey="config.google.includeThoughts"
			label="Include Thoughts"
			options={["true", "false"]}
		/>
		<InputField
			{...props}
			fieldKey="config.google.cachedContent"
			label="Cached Content"
			placeholder="cached-contents/abc123"
		/>
		<JSONField
			{...props}
			fieldKey="config.google.safetySettingsJSON"
			label="Safety Settings JSON"
			placeholder={`[
  {"category":"HARM_CATEGORY_DANGEROUS_CONTENT","threshold":"BLOCK_ONLY_HIGH"}
]`}
		/>
	</div>
);

const OpenAICompatFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
		<SelectField
			{...props}
			fieldKey="config.openaicompat.reasoningEffort"
			label="Reasoning Effort"
			options={modelConfigReasoningEffortOptions}
		/>
		<InputField
			{...props}
			fieldKey="config.openaicompat.user"
			label="User"
			placeholder="end-user-id"
		/>
	</div>
);

const OpenRouterFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
		<SelectField
			{...props}
			fieldKey="config.openrouter.reasoningEnabled"
			label="Reasoning Enabled"
			options={["true", "false"]}
		/>
		<SelectField
			{...props}
			fieldKey="config.openrouter.reasoningEffort"
			label="Reasoning Effort"
			options={modelConfigReasoningEffortOptions}
		/>
		<InputField
			{...props}
			fieldKey="config.openrouter.reasoningMaxTokens"
			label="Reasoning Max Tokens"
			placeholder="2048"
		/>
		<SelectField
			{...props}
			fieldKey="config.openrouter.reasoningExclude"
			label="Reasoning Exclude"
			options={["true", "false"]}
		/>
		<SelectField
			{...props}
			fieldKey="config.openrouter.parallelToolCalls"
			label="Parallel Tool Calls"
			options={["true", "false"]}
		/>
		<SelectField
			{...props}
			fieldKey="config.openrouter.includeUsage"
			label="Include Usage"
			options={["true", "false"]}
		/>
		<InputField
			{...props}
			fieldKey="config.openrouter.user"
			label="User"
			placeholder="end-user-id"
		/>
	</div>
);

const VercelFields: FC<FieldRenderContext> = (props) => (
	<div className="grid min-w-0 gap-3 sm:grid-cols-2">
		<SelectField
			{...props}
			fieldKey="config.vercel.reasoningEnabled"
			label="Reasoning Enabled"
			options={["true", "false"]}
		/>
		<SelectField
			{...props}
			fieldKey="config.vercel.reasoningEffort"
			label="Reasoning Effort"
			options={modelConfigReasoningEffortOptions}
		/>
		<InputField
			{...props}
			fieldKey="config.vercel.reasoningMaxTokens"
			label="Reasoning Max Tokens"
			placeholder="2048"
		/>
		<SelectField
			{...props}
			fieldKey="config.vercel.reasoningExclude"
			label="Reasoning Exclude"
			options={["true", "false"]}
		/>
		<SelectField
			{...props}
			fieldKey="config.vercel.parallelToolCalls"
			label="Parallel Tool Calls"
			options={["true", "false"]}
		/>
		<InputField
			{...props}
			fieldKey="config.vercel.user"
			label="User"
			placeholder="end-user-id"
		/>
	</div>
);

// ── Main component ─────────────────────────────────────────────

type ModelConfigFieldsProps = {
	provider: string;
	form: FormikContextType<ModelFormValues>;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	disabled: boolean;
};

const renderProviderSpecificFields = (
	normalized: string,
	ctx: FieldRenderContext,
) => {
	switch (normalized) {
		case "openai":
			return <OpenAIFields {...ctx} />;
		case "azure":
			return <OpenAIFields {...ctx} />;
		case "anthropic":
			return <AnthropicFields {...ctx} />;
		case "bedrock":
			return <AnthropicFields {...ctx} />;
		case "google":
			return <GoogleFields {...ctx} />;
		case "openaicompat":
			return <OpenAICompatFields {...ctx} />;
		case "openrouter":
			return <OpenRouterFields {...ctx} />;
		case "vercel":
			return <VercelFields {...ctx} />;
		default:
			return null;
	}
};

/**
 * Provider-specific fields (reasoning, tool calls, etc.) that
 * should be visible at the top level of the model form.
 */
export const ModelConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	disabled,
}) => {
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };
	const normalized = normalizeProvider(provider);
	const providerFields = renderProviderSpecificFields(normalized, ctx);

	if (!providerFields) {
		return null;
	}

	return providerFields;
};

/**
 * General model config fields (max output tokens, temperature,
 * top P, etc.) intended to be shown under an "Advanced" section.
 */
export const GeneralModelConfigFields: FC<ModelConfigFieldsProps> = ({
	form,
	fieldErrors,
	disabled,
}) => {
	const ctx: FieldRenderContext = { form, fieldErrors, disabled };

	return (
		<>
			<InputField
				{...ctx}
				fieldKey="config.maxOutputTokens"
				label="Max Output Tokens"
				placeholder="32000"
			/>
			<InputField
				{...ctx}
				fieldKey="config.temperature"
				label="Temperature"
				placeholder="0.2"
			/>
			<InputField
				{...ctx}
				fieldKey="config.topP"
				label="Top P"
				placeholder="0.95"
			/>
			<InputField
				{...ctx}
				fieldKey="config.topK"
				label="Top K"
				placeholder="40"
			/>
			<InputField
				{...ctx}
				fieldKey="config.presencePenalty"
				label="Presence Penalty"
				placeholder="0"
			/>
			<InputField
				{...ctx}
				fieldKey="config.frequencyPenalty"
				label="Frequency Penalty"
				placeholder="0"
			/>
		</>
	);
};
