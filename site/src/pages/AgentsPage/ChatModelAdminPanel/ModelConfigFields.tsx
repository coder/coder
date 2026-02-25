import { Input } from "components/Input/Input";
import { Textarea } from "components/Textarea/Textarea";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import type { FC } from "react";
import { cn } from "utils/cn";
import type { ModelConfigFormBuildResult, ModelConfigFormState } from "./modelConfigFormLogic";

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
	inputIdPrefix: string;
	form: ModelConfigFormState;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	onChange: (key: keyof ModelConfigFormState, value: string) => void;
	disabled: boolean;
};

const InputField: FC<
	FieldRenderContext & {
		fieldKey: keyof ModelConfigFormState;
		label: string;
		placeholder: string;
	}
> = ({ inputIdPrefix, form, fieldErrors, onChange, disabled, fieldKey, label, placeholder }) => {
	const fieldID = `${inputIdPrefix}-${fieldKey}`;
	const fieldError = fieldErrors[fieldKey];
	return (
		<div className="grid gap-1.5">
			<label
				htmlFor={fieldID}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}{" "}
				<span className="font-normal text-content-secondary">
					(optional)
				</span>
			</label>
			<Input
				id={fieldID}
				className={cn(
					"h-10 text-[13px]",
					fieldError && "border-content-destructive",
				)}
				placeholder={placeholder}
				value={form[fieldKey]}
				onChange={(e) => onChange(fieldKey, e.target.value)}
				disabled={disabled}
			/>
			{fieldError && (
				<p className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

const SelectField: FC<
	FieldRenderContext & {
		fieldKey: keyof ModelConfigFormState;
		label: string;
		options: readonly string[];
	}
> = ({ inputIdPrefix, form, fieldErrors, onChange, disabled, fieldKey, label, options }) => {
	const fieldID = `${inputIdPrefix}-${fieldKey}`;
	const fieldError = fieldErrors[fieldKey];
	return (
		<div className="grid gap-1.5">
			<label
				htmlFor={fieldID}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}{" "}
				<span className="font-normal text-content-secondary">
					(optional)
				</span>
			</label>
			<Select
				value={form[fieldKey] || unsetSelectValue}
				onValueChange={(value) =>
					onChange(
						fieldKey,
						value === unsetSelectValue ? "" : value,
					)
				}
				disabled={disabled}
			>
				<SelectTrigger
					id={fieldID}
					className={cn(
						"h-10 text-[13px]",
						fieldError && "border-content-destructive",
					)}
				>
					<SelectValue placeholder="Use backend default" />
				</SelectTrigger>
				<SelectContent>
					<SelectItem value={unsetSelectValue}>
						Use backend default
					</SelectItem>
					{options.map((option) => (
						<SelectItem key={option} value={option}>
							{option}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
			{fieldError && (
				<p className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

const JSONField: FC<
	FieldRenderContext & {
		fieldKey: keyof ModelConfigFormState;
		label: string;
		placeholder: string;
	}
> = ({ inputIdPrefix, form, fieldErrors, onChange, disabled, fieldKey, label, placeholder }) => {
	const fieldID = `${inputIdPrefix}-${fieldKey}`;
	const fieldError = fieldErrors[fieldKey];
	return (
		<div className="grid gap-1.5">
			<label
				htmlFor={fieldID}
				className="text-[13px] font-medium text-content-primary"
			>
				{label}{" "}
				<span className="font-normal text-content-secondary">
					(optional)
				</span>
			</label>
			<Textarea
				id={fieldID}
				className={cn(
					"min-h-[96px] font-mono text-xs",
					fieldError && "border-content-destructive",
				)}
				placeholder={placeholder}
				value={form[fieldKey]}
				onChange={(e) => onChange(fieldKey, e.target.value)}
				disabled={disabled}
			/>
			{fieldError && (
				<p className="m-0 text-xs text-content-destructive">
					{fieldError}
				</p>
			)}
		</div>
	);
};

// ── Provider-specific field sets ───────────────────────────────

const OpenAIFields: FC<FieldRenderContext & { sectionTitle: string }> = (
	props,
) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			{props.sectionTitle}
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="openaiReasoningEffort"
				label="Reasoning effort"
				options={modelConfigReasoningEffortOptions}
			/>
			<SelectField
				{...props}
				fieldKey="openaiParallelToolCalls"
				label="Parallel tool calls"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="openaiTextVerbosity"
				label="Text verbosity"
				options={modelConfigTextVerbosityOptions}
			/>
			<InputField
				{...props}
				fieldKey="openaiServiceTier"
				label="Service tier"
				placeholder="auto"
			/>
			<InputField
				{...props}
				fieldKey="openaiReasoningSummary"
				label="Reasoning summary"
				placeholder="detailed"
			/>
			<InputField
				{...props}
				fieldKey="openaiUser"
				label="User"
				placeholder="end-user-id"
			/>
		</div>
	</div>
);

const AnthropicFields: FC<FieldRenderContext & { sectionTitle: string }> = (
	props,
) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			{props.sectionTitle}
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="anthropicEffort"
				label="Output effort"
				options={modelConfigAnthropicEffortOptions}
			/>
			<InputField
				{...props}
				fieldKey="anthropicThinkingBudgetTokens"
				label="Thinking budget tokens"
				placeholder="4000"
			/>
			<SelectField
				{...props}
				fieldKey="anthropicSendReasoning"
				label="Send reasoning"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="anthropicDisableParallelToolUse"
				label="Disable parallel tool use"
				options={["true", "false"]}
			/>
		</div>
	</div>
);

const GoogleFields: FC<FieldRenderContext> = (props) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			Google options
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<InputField
				{...props}
				fieldKey="googleThinkingBudget"
				label="Thinking budget"
				placeholder="1024"
			/>
			<SelectField
				{...props}
				fieldKey="googleIncludeThoughts"
				label="Include thoughts"
				options={["true", "false"]}
			/>
			<InputField
				{...props}
				fieldKey="googleCachedContent"
				label="Cached content"
				placeholder="cached-contents/abc123"
			/>
			<JSONField
				{...props}
				fieldKey="googleSafetySettingsJSON"
				label="Safety settings JSON"
				placeholder={`[
  {"category":"HARM_CATEGORY_DANGEROUS_CONTENT","threshold":"BLOCK_ONLY_HIGH"}
]`}
			/>
		</div>
	</div>
);

const OpenAICompatFields: FC<FieldRenderContext> = (props) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			OpenAI-compatible options
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="openAICompatReasoningEffort"
				label="Reasoning effort"
				options={modelConfigReasoningEffortOptions}
			/>
			<InputField
				{...props}
				fieldKey="openAICompatUser"
				label="User"
				placeholder="end-user-id"
			/>
		</div>
	</div>
);

const OpenRouterFields: FC<FieldRenderContext> = (props) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			OpenRouter options
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="openrouterReasoningEnabled"
				label="Reasoning enabled"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="openrouterReasoningEffort"
				label="Reasoning effort"
				options={modelConfigReasoningEffortOptions}
			/>
			<InputField
				{...props}
				fieldKey="openrouterReasoningMaxTokens"
				label="Reasoning max tokens"
				placeholder="2048"
			/>
			<SelectField
				{...props}
				fieldKey="openrouterReasoningExclude"
				label="Reasoning exclude"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="openrouterParallelToolCalls"
				label="Parallel tool calls"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="openrouterIncludeUsage"
				label="Include usage"
				options={["true", "false"]}
			/>
			<InputField
				{...props}
				fieldKey="openrouterUser"
				label="User"
				placeholder="end-user-id"
			/>
		</div>
	</div>
);

const VercelFields: FC<FieldRenderContext> = (props) => (
	<div className="space-y-2">
		<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
			Vercel options
		</p>
		<div className="grid gap-3 md:grid-cols-2">
			<SelectField
				{...props}
				fieldKey="vercelReasoningEnabled"
				label="Reasoning enabled"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="vercelReasoningEffort"
				label="Reasoning effort"
				options={modelConfigReasoningEffortOptions}
			/>
			<InputField
				{...props}
				fieldKey="vercelReasoningMaxTokens"
				label="Reasoning max tokens"
				placeholder="2048"
			/>
			<SelectField
				{...props}
				fieldKey="vercelReasoningExclude"
				label="Reasoning exclude"
				options={["true", "false"]}
			/>
			<SelectField
				{...props}
				fieldKey="vercelParallelToolCalls"
				label="Parallel tool calls"
				options={["true", "false"]}
			/>
			<InputField
				{...props}
				fieldKey="vercelUser"
				label="User"
				placeholder="end-user-id"
			/>
		</div>
	</div>
);

// ── Main component ─────────────────────────────────────────────

type ModelConfigFieldsProps = {
	provider: string;
	form: ModelConfigFormState;
	fieldErrors: ModelConfigFormBuildResult["fieldErrors"];
	onChange: (key: keyof ModelConfigFormState, value: string) => void;
	disabled: boolean;
	inputIdPrefix: string;
};

export const ModelConfigFields: FC<ModelConfigFieldsProps> = ({
	provider,
	form,
	fieldErrors,
	onChange,
	disabled,
	inputIdPrefix,
}) => {
	const ctx: FieldRenderContext = {
		inputIdPrefix,
		form,
		fieldErrors,
		onChange,
		disabled,
	};
	const normalized = provider.trim().toLowerCase();

	const renderProviderSpecificFields = () => {
		switch (normalized) {
			case "openai":
				return <OpenAIFields {...ctx} sectionTitle="OpenAI options" />;
			case "azure":
				return (
					<OpenAIFields
						{...ctx}
						sectionTitle="OpenAI options (Azure)"
					/>
				);
			case "anthropic":
				return (
					<AnthropicFields
						{...ctx}
						sectionTitle="Anthropic options"
					/>
				);
			case "bedrock":
				return (
					<AnthropicFields
						{...ctx}
						sectionTitle="Anthropic options (Bedrock)"
					/>
				);
			case "google":
				return <GoogleFields {...ctx} />;
			case "openaicompat":
				return <OpenAICompatFields {...ctx} />;
			case "openrouter":
				return <OpenRouterFields {...ctx} />;
			case "vercel":
				return <VercelFields {...ctx} />;
			default:
				return (
					<p className="m-0 text-xs text-content-secondary">
						No provider-specific options are available for this
						provider.
					</p>
				);
		}
	};

	return (
		<div className="space-y-2">
			<p className="m-0 text-[13px] font-medium text-content-primary">
				Model call config{" "}
				<span className="font-normal text-content-secondary">
					(optional)
				</span>
			</p>
			<div className="space-y-2">
				<p className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
					General options
				</p>
				<div className="grid gap-3 md:grid-cols-2">
					<InputField
						{...ctx}
						fieldKey="maxOutputTokens"
						label="Max output tokens"
						placeholder="32000"
					/>
					<InputField
						{...ctx}
						fieldKey="temperature"
						label="Temperature"
						placeholder="0.2"
					/>
					<InputField
						{...ctx}
						fieldKey="topP"
						label="Top P"
						placeholder="0.95"
					/>
					<InputField
						{...ctx}
						fieldKey="topK"
						label="Top K"
						placeholder="40"
					/>
					<InputField
						{...ctx}
						fieldKey="presencePenalty"
						label="Presence penalty"
						placeholder="0"
					/>
					<InputField
						{...ctx}
						fieldKey="frequencyPenalty"
						label="Frequency penalty"
						placeholder="0"
					/>
				</div>
			</div>
			{renderProviderSpecificFields()}
		</div>
	);
};
