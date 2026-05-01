import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useId, useRef } from "react";
import { getErrorMessage } from "#/api/errors";
import type {
	AdvisorConfig,
	ChatModelConfig,
	UpdateAdvisorConfigRequest,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
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

const nilUUID = "00000000-0000-0000-0000-000000000000";
const advisorReasoningEfforts = ["", "low", "medium", "high"] as const;
type AdvisorReasoningEffort = (typeof advisorReasoningEfforts)[number];
const chatModelFallbackValue = "__use-chat-model__";
const unavailableModelValue = "__unavailable-model__";
const chatReasoningFallbackValue = "__use-chat-reasoning__";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AdvisorSettingsProps {
	advisorConfigData: AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	modelConfigs: readonly ChatModelConfig[];
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	isFetchingModelConfigs: boolean;
	onSaveAdvisorConfig: (
		req: UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
}

type AdvisorSettingsFormValues = {
	enabled: boolean;
	max_uses_per_run: string;
	max_output_tokens: string;
	reasoning_effort: AdvisorReasoningEffort;
	model_config_id: string;
};

const isUnsetModelConfigId = (id: string): boolean =>
	id === "" || id === nilUUID;

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
		max_uses_per_run: String(
			normalizeNonNegativeInteger(config?.max_uses_per_run),
		),
		max_output_tokens: String(
			normalizeNonNegativeInteger(config?.max_output_tokens),
		),
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
		? nilUUID
		: values.model_config_id,
});

const isNonNegativeIntegerString = (value: string): boolean => {
	if (value.trim() === "") {
		return false;
	}
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed >= 0 && Number.isInteger(parsed);
};

const validateAdvisorConfig = (values: AdvisorSettingsFormValues) => {
	const errors: Partial<Record<keyof AdvisorSettingsFormValues, string>> = {};

	// Skip validation of the advisor-only fields when the feature is disabled.
	// Those inputs are hidden, so an admin disabling the advisor should not be
	// blocked by stale invalid values left in hidden fields.
	if (!values.enabled) {
		return errors;
	}

	if (!isNonNegativeIntegerString(values.max_uses_per_run)) {
		errors.max_uses_per_run =
			"Max uses per run must be a non-negative integer.";
	}

	if (!isNonNegativeIntegerString(values.max_output_tokens)) {
		errors.max_output_tokens =
			"Max output tokens must be a non-negative integer.";
	}

	if (!isAdvisorReasoningEffort(values.reasoning_effort)) {
		errors.reasoning_effort = "Select a valid reasoning effort.";
	}

	return errors;
};

const getModelDisplayName = (config: ChatModelConfig): string =>
	config.display_name.trim() || config.model;

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
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	modelConfigs,
	modelConfigsError,
	isLoadingModelConfigs,
	isFetchingModelConfigs,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
}) => {
	const maxUsesId = useId();
	const maxOutputTokensId = useId();
	const hasLoadedAdvisorConfig = advisorConfigData !== undefined;
	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);

	// Track the most recent committed advisor values (the server's view or the
	// last successful save). Reading `advisorConfigData` directly in `onSubmit`
	// can yield a stale snapshot when a refetch is in flight or has failed,
	// which would silently roll back recently saved limits if the user then
	// disables the advisor before the query settles.
	const committedValuesRef = useRef<AdvisorSettingsFormValues>(
		normalizeAdvisorConfig(advisorConfigData),
	);
	useEffect(() => {
		committedValuesRef.current = normalizeAdvisorConfig(advisorConfigData);
	}, [advisorConfigData]);

	const form = useFormik<AdvisorSettingsFormValues>({
		enableReinitialize: true,
		validateOnMount: true,
		initialValues: normalizeAdvisorConfig(advisorConfigData),
		validate: validateAdvisorConfig,
		onSubmit: (values, { resetForm }) => {
			// When disabling, preserve the last committed values for the hidden
			// fields so potentially invalid in-flight edits (empty strings,
			// fractional numbers) cannot silently overwrite previously
			// configured limits, and so a pending or failed refetch of the
			// advisor config cannot revert recently saved values.
			let source: AdvisorSettingsFormValues = values.enabled
				? values
				: { ...committedValuesRef.current, enabled: false };
			// If the last committed model override references a model config
			// that no longer exists, the backend rejects the stale ID with a
			// 400. When disabling, clear the override so a simple disable
			// stays reliable in that edge case; the override is unusable
			// anyway and the admin will reselect one on re-enable. Only scrub
			// when model configs have loaded successfully and no refetch is in
			// flight: during an initial load, a background refetch, or on
			// error we cannot distinguish "truly missing" from "not loaded
			// yet", and deciding from stale cache could either preserve a
			// now-deleted ID (causing a 400 on disable/save) or silently drop
			// an override that is actually still valid but missing from a
			// stale cache. `isLoading` alone is insufficient because
			// react-query keeps it false during background refetches when
			// cached data already exists, so `isFetching` covers that gap. An
			// empty list after a successful load is a definitive answer, so
			// the scrub still fires (covers the recovery case where every
			// model config has been deleted).
			if (
				!source.enabled &&
				!isUnsetModelConfigId(source.model_config_id) &&
				!isLoadingModelConfigs &&
				!isFetchingModelConfigs &&
				!modelConfigsError &&
				!modelConfigs.some((config) => config.id === source.model_config_id)
			) {
				source = { ...source, model_config_id: "" };
			}
			const request = toAdvisorConfigRequest(source);
			onSaveAdvisorConfig(request, {
				onSuccess: () => {
					const nextValues = normalizeAdvisorConfig(request);
					committedValuesRef.current = nextValues;
					resetForm({ values: nextValues });
				},
			});
		},
	});

	const isFormDisabled =
		isSavingAdvisorConfig ||
		isAdvisorConfigLoading ||
		isAdvisorConfigFetching ||
		!hasLoadedAdvisorConfig;
	const isModelSelectDisabled =
		isFormDisabled || isLoadingModelConfigs || Boolean(modelConfigsError);
	const hasUnavailableSelectedModel =
		!isLoadingModelConfigs &&
		!isUnsetModelConfigId(form.values.model_config_id) &&
		!enabledModelConfigs.some(
			(config) => config.id === form.values.model_config_id,
		);
	const selectedModelConfig = modelConfigs.find(
		(config) => config.id === form.values.model_config_id,
	);
	const selectedModelLabel = isUnsetModelConfigId(form.values.model_config_id)
		? "Use chat model"
		: isLoadingModelConfigs
			? "Loading..."
			: selectedModelConfig
				? getModelDisplayName(selectedModelConfig)
				: `Unavailable model (${form.values.model_config_id})`;
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
				: "Model overrides are unavailable. The current selection will be sent unchanged."
			: "Choose a dedicated advisor model, or leave this unset to reuse the chat model.";

	return (
		<form className="space-y-3" onSubmit={form.handleSubmit}>
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-sm font-semibold text-content-primary">
					Advisor
				</h3>
				<Badge size="sm" variant="warning" className="cursor-default">
					<TriangleAlertIcon className="h-3 w-3" />
					Experimental feature
				</Badge>
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
						<Label htmlFor={maxUsesId} className="text-xs text-content-primary">
							Max uses per run
						</Label>
						<Input
							id={maxUsesId}
							name="max_uses_per_run"
							type="number"
							min={0}
							step={1}
							inputMode="numeric"
							aria-label="Max uses per run"
							value={form.values.max_uses_per_run}
							// Bypass Formik's `handleChange` on purpose: for `type="number"`
							// it parses the raw input with `parseFloat` and replaces the
							// declared `string` form value with a `number`, which would
							// break string-only validators like `isNonNegativeIntegerString`.
							onChange={(event) =>
								void form.setFieldValue(
									"max_uses_per_run",
									event.currentTarget.value,
								)
							}
							onBlur={form.handleBlur}
							aria-invalid={Boolean(form.errors.max_uses_per_run)}
							disabled={isFormDisabled}
							className="h-9 bg-surface-primary text-[13px]"
						/>
						<p className="m-0 text-xs text-content-secondary">
							Set to 0 to leave the per-run call count unlimited.
						</p>
					</div>

					<div className="space-y-1.5">
						<Label
							htmlFor={maxOutputTokensId}
							className="text-xs text-content-primary"
						>
							Max output tokens
						</Label>
						<Input
							id={maxOutputTokensId}
							name="max_output_tokens"
							type="number"
							min={0}
							step={1}
							inputMode="numeric"
							aria-label="Max output tokens"
							value={form.values.max_output_tokens}
							// See `max_uses_per_run` above for why `handleChange` is
							// bypassed: Formik's `type="number"` coercion would replace
							// the declared `string` form value with a `number`.
							onChange={(event) =>
								void form.setFieldValue(
									"max_output_tokens",
									event.currentTarget.value,
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
							Controls how hard the advisor model reasons before responding.
							Leave unset to use the model's default.
						</p>
					</div>

					<div className="space-y-1.5">
						<Label className="text-xs text-content-primary">
							Advisor model
						</Label>
						<Select
							value={selectedModelValue}
							onValueChange={(value) => {
								if (value === chatModelFallbackValue) {
									void form.setFieldValue("model_config_id", "");
									return;
								}
								if (value === unavailableModelValue) {
									return;
								}
								void form.setFieldValue("model_config_id", value);
							}}
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
										{getModelDisplayName(config)}
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
					{getErrorMessage(
						saveAdvisorConfigError,
						"Failed to save advisor settings.",
					)}
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
