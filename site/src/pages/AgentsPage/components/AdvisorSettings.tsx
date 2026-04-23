import { useFormik } from "formik";
import type { FC } from "react";
import type {
	AdvisorConfig,
	ChatModelConfig,
	UpdateAdvisorConfigRequest,
} from "#/api/typesGenerated";

type AdvisorReasoningEffort = "" | "low" | "medium" | "high";

import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Switch } from "#/components/Switch/Switch";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AdvisorSettingsProps {
	advisorConfigData: AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigLoadError: boolean;
	modelConfigs: readonly ChatModelConfig[];
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveAdvisorConfig: (
		req: UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
}

type AdvisorSettingsFormValues = {
	enabled: boolean;
	max_uses_per_run: number;
	max_output_tokens: number;
	reasoning_effort: AdvisorReasoningEffort;
	model_config_id: string;
};

const ZERO_UUID = "00000000-0000-0000-0000-000000000000";
const advisorReasoningEfforts = ["", "low", "medium", "high"] as const;
const chatModelFallbackValue = "__use-chat-model__";
const unavailableModelValue = "__unavailable-model__";
const chatReasoningFallbackValue = "__use-chat-reasoning__";

const isUnsetModelConfigId = (id: string): boolean =>
	id === "" || id === ZERO_UUID;

const isAdvisorReasoningEffort = (
	value: string,
): value is AdvisorReasoningEffort => {
	return advisorReasoningEfforts.includes(value as AdvisorReasoningEffort);
};

const normalizeNonNegativeInteger = (
	value: number | string | undefined,
): number => {
	const parsed = typeof value === "number" ? value : Number(value);
	if (!Number.isFinite(parsed) || parsed < 0) {
		return 0;
	}
	return Math.trunc(parsed);
};

const normalizeAdvisorConfig = (
	config: AdvisorConfig | undefined,
): AdvisorSettingsFormValues => {
	const reasoningEffort = config?.reasoning_effort ?? "";
	return {
		enabled: config?.enabled ?? false,
		max_uses_per_run: normalizeNonNegativeInteger(config?.max_uses_per_run),
		max_output_tokens: normalizeNonNegativeInteger(config?.max_output_tokens),
		reasoning_effort: isAdvisorReasoningEffort(reasoningEffort)
			? reasoningEffort
			: "",
		model_config_id:
			typeof config?.model_config_id === "string" &&
			!isUnsetModelConfigId(config.model_config_id)
				? config.model_config_id
				: "",
	};
};

const toAdvisorConfigRequest = (
	values: AdvisorSettingsFormValues,
): UpdateAdvisorConfigRequest => ({
	enabled: values.enabled,
	max_uses_per_run: normalizeNonNegativeInteger(values.max_uses_per_run),
	max_output_tokens: normalizeNonNegativeInteger(values.max_output_tokens),
	reasoning_effort: values.reasoning_effort,
	model_config_id: isUnsetModelConfigId(values.model_config_id)
		? ZERO_UUID
		: values.model_config_id,
});

const validateAdvisorConfig = (values: AdvisorSettingsFormValues) => {
	const errors: Partial<Record<keyof AdvisorSettingsFormValues, string>> = {};

	if (
		!Number.isInteger(values.max_uses_per_run) ||
		values.max_uses_per_run < 0
	) {
		errors.max_uses_per_run =
			"Max uses per run must be a non-negative integer.";
	}

	if (
		!Number.isInteger(values.max_output_tokens) ||
		values.max_output_tokens < 0
	) {
		errors.max_output_tokens =
			"Max output tokens must be a non-negative integer.";
	}

	if (!isAdvisorReasoningEffort(values.reasoning_effort)) {
		errors.reasoning_effort = "Select a valid reasoning effort.";
	}

	return errors;
};

const getReasoningEffortLabel = (value: AdvisorReasoningEffort): string => {
	switch (value) {
		case "low":
			return "Low";
		case "medium":
			return "Medium";
		case "high":
			return "High";
		default:
			return "Use chat model default";
	}
};

export const AdvisorSettings: FC<AdvisorSettingsProps> = ({
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigLoadError,
	modelConfigs,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
}) => {
	const hasLoadedAdvisorConfig = advisorConfigData !== undefined;
	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);

	const form = useFormik<AdvisorSettingsFormValues>({
		enableReinitialize: true,
		validateOnMount: true,
		initialValues: normalizeAdvisorConfig(advisorConfigData),
		validate: validateAdvisorConfig,
		onSubmit: (values, { resetForm }) => {
			const request = toAdvisorConfigRequest(values);
			onSaveAdvisorConfig(request, {
				onSuccess: () => {
					resetForm({ values: normalizeAdvisorConfig(request) });
				},
			});
		},
	});

	const isFormDisabled =
		isSavingAdvisorConfig || isAdvisorConfigLoading || !hasLoadedAdvisorConfig;
	const isModelSelectDisabled =
		isFormDisabled || isLoadingModelConfigs || Boolean(modelConfigsError);
	const hasUnavailableSelectedModel =
		!isUnsetModelConfigId(form.values.model_config_id) &&
		!enabledModelConfigs.some(
			(config) => config.id === form.values.model_config_id,
		);
	const selectedModelConfig = modelConfigs.find(
		(config) => config.id === form.values.model_config_id,
	);
	const selectedModelLabel = isUnsetModelConfigId(form.values.model_config_id)
		? "Use chat model"
		: (selectedModelConfig?.display_name ??
			`Unavailable model (${form.values.model_config_id})`);
	const selectedModelValue = isUnsetModelConfigId(form.values.model_config_id)
		? chatModelFallbackValue
		: hasUnavailableSelectedModel
			? unavailableModelValue
			: form.values.model_config_id;
	const modelHelperText = isLoadingModelConfigs
		? "Loading chat model overrides."
		: modelConfigsError
			? isUnsetModelConfigId(form.values.model_config_id)
				? "Model overrides are unavailable. Saving will keep using the chat model."
				: "Model overrides are unavailable. Your current advisor model will be preserved on save."
			: "Choose a dedicated advisor model, or leave this unset to reuse the chat model.";

	return (
		<form className="space-y-3" onSubmit={form.handleSubmit}>
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-sm font-semibold text-content-primary">
					Advisor
				</h3>
			</div>
			<div className="flex items-center justify-between gap-4">
				<div className="!mt-0.5 m-0 flex-1 space-y-2 text-xs text-content-secondary">
					<p className="m-0">
						Allow root agent chats to call the advisor tool for strategic
						guidance.
					</p>
					<p className="m-0">
						When enabled, you can cap advisor usage per run and optionally use
						an override model.
					</p>
				</div>
				<Switch
					checked={form.values.enabled}
					onCheckedChange={(checked) =>
						void form.setFieldValue("enabled", checked)
					}
					aria-label="Enable advisor"
					disabled={isFormDisabled}
				/>
			</div>

			{form.values.enabled && (
				<div className="grid gap-4 rounded-lg border border-border bg-surface-secondary p-4 md:grid-cols-2">
					<div className="space-y-1.5">
						<Label
							htmlFor="advisor-max-uses"
							className="text-xs text-content-primary"
						>
							Max uses per run
						</Label>
						<Input
							id="advisor-max-uses"
							type="number"
							min={0}
							step={1}
							inputMode="numeric"
							aria-label="Max uses per run"
							value={form.values.max_uses_per_run}
							onChange={(event) =>
								void form.setFieldValue(
									"max_uses_per_run",
									normalizeNonNegativeInteger(event.currentTarget.value),
								)
							}
							onBlur={form.handleBlur}
							aria-invalid={Boolean(form.errors.max_uses_per_run)}
							disabled={isFormDisabled}
							className="h-9 bg-surface-primary text-[13px]"
						/>
						<p className="m-0 text-xs text-content-secondary">
							Set to 0 for unlimited advisor calls within a run.
						</p>
					</div>

					<div className="space-y-1.5">
						<Label
							htmlFor="advisor-max-output-tokens"
							className="text-xs text-content-primary"
						>
							Max output tokens
						</Label>
						<Input
							id="advisor-max-output-tokens"
							type="number"
							min={0}
							step={1}
							inputMode="numeric"
							aria-label="Max output tokens"
							value={form.values.max_output_tokens}
							onChange={(event) =>
								void form.setFieldValue(
									"max_output_tokens",
									normalizeNonNegativeInteger(event.currentTarget.value),
								)
							}
							onBlur={form.handleBlur}
							aria-invalid={Boolean(form.errors.max_output_tokens)}
							disabled={isFormDisabled}
							className="h-9 bg-surface-primary text-[13px]"
						/>
						<p className="m-0 text-xs text-content-secondary">
							Set to 0 to use the server default output limit.
						</p>
					</div>

					<div className="space-y-1.5">
						<Label className="text-xs text-content-primary">
							Reasoning effort
						</Label>
						<Select
							value={form.values.reasoning_effort || chatReasoningFallbackValue}
							onValueChange={(value) =>
								void form.setFieldValue(
									"reasoning_effort",
									value === chatReasoningFallbackValue ? "" : value,
								)
							}
							disabled={isFormDisabled}
						>
							<SelectTrigger
								className="h-9 bg-surface-primary text-[13px]"
								aria-label="Reasoning effort"
							>
								<SelectValue placeholder="Use chat model default">
									{getReasoningEffortLabel(form.values.reasoning_effort)}
								</SelectValue>
							</SelectTrigger>
							<SelectContent>
								<SelectItem value={chatReasoningFallbackValue}>
									Use chat model default
								</SelectItem>
								<SelectItem value="low">Low</SelectItem>
								<SelectItem value="medium">Medium</SelectItem>
								<SelectItem value="high">High</SelectItem>
							</SelectContent>
						</Select>
						<p className="m-0 text-xs text-content-secondary">
							Override the advisor's reasoning effort for nested runs.
						</p>
					</div>

					<div className="space-y-1.5">
						<Label className="text-xs text-content-primary">
							Advisor model
						</Label>
						<Select
							value={selectedModelValue}
							onValueChange={(value) =>
								void form.setFieldValue(
									"model_config_id",
									value === chatModelFallbackValue ? "" : value,
								)
							}
							disabled={isModelSelectDisabled}
						>
							<SelectTrigger
								className="h-9 bg-surface-primary text-[13px]"
								aria-label="Advisor model"
							>
								<SelectValue placeholder="Use chat model">
									{selectedModelLabel}
								</SelectValue>
							</SelectTrigger>
							<SelectContent>
								{hasUnavailableSelectedModel && (
									<SelectItem value={unavailableModelValue}>
										{selectedModelLabel}
									</SelectItem>
								)}
								<SelectItem value={chatModelFallbackValue}>
									Use chat model
								</SelectItem>
								{enabledModelConfigs.map((config) => (
									<SelectItem key={config.id} value={config.id}>
										{config.display_name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						<p className="m-0 text-xs text-content-secondary">
							{modelHelperText}
						</p>
					</div>
				</div>
			)}

			<div className="flex justify-end">
				<Button
					size="sm"
					type="submit"
					disabled={isFormDisabled || !form.dirty || !form.isValid}
				>
					Save
				</Button>
			</div>

			{isSaveAdvisorConfigError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save advisor settings.
				</p>
			)}
			{isAdvisorConfigLoadError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load advisor settings.
				</p>
			)}
		</form>
	);
};
