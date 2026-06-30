import { useFormik } from "formik";
import { type FC, useEffect, useId, useRef } from "react";
import { getErrorMessage } from "#/api/errors";
import type {
	AdvisorConfig,
	ChatModelConfig,
	UpdateAdvisorConfigRequest,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { AgentSettingLayout } from "#/pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout";

const nilUUID = "00000000-0000-0000-0000-000000000000";
const chatModelFallbackValue = "__use-chat-model__";
const unavailableModelValue = "__unavailable-model__";

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
	max_uses_per_run: string;
	max_output_tokens: string;
	model_config_id: string;
};

const isUnsetModelConfigId = (id: string): boolean =>
	id === "" || id === nilUUID;

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
): AdvisorSettingsFormValues => ({
	max_uses_per_run: String(
		normalizeNonNegativeInteger(config?.max_uses_per_run),
	),
	max_output_tokens: String(
		normalizeNonNegativeInteger(config?.max_output_tokens),
	),
	model_config_id:
		typeof config?.model_config_id === "string" &&
		!isUnsetModelConfigId(config.model_config_id)
			? config.model_config_id
			: "",
});

const toAdvisorConfigRequest = (
	values: AdvisorSettingsFormValues,
): UpdateAdvisorConfigRequest => ({
	enabled: true,
	max_uses_per_run: normalizeNonNegativeInteger(values.max_uses_per_run),
	max_output_tokens: normalizeNonNegativeInteger(values.max_output_tokens),
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

	if (!isNonNegativeIntegerString(values.max_uses_per_run)) {
		errors.max_uses_per_run =
			"Max uses per run must be a non-negative integer.";
	}

	if (!isNonNegativeIntegerString(values.max_output_tokens)) {
		errors.max_output_tokens =
			"Max output tokens must be a non-negative integer.";
	}

	return errors;
};

const getModelDisplayName = (config: ChatModelConfig): string =>
	config.display_name.trim() || config.model;

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
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const hasLoadedAdvisorConfig = advisorConfigData !== undefined;
	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);

	// Track the most recent committed advisor values (the server's view or the
	// last successful save). Reading `advisorConfigData` directly in `onSubmit`
	// can yield a stale snapshot when a refetch is in flight or has failed.
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
			// If the last committed model override references a model config
			// that no longer exists, the backend rejects the stale ID with a
			// 400. Clear the override so a save stays reliable in that edge
			// case. Only scrub when model configs have loaded successfully and
			// no refetch is in flight.
			let source = values;
			if (
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
					showSavedState();
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
	const canSave = hasLoadedAdvisorConfig && form.dirty && form.isValid;

	return (
		<AgentSettingLayout
			title="Advisor"
			description="Cap advisor usage per run and optionally use an override model. The advisor provides strategic guidance to root agent chats."
			showSave={canSave}
			isSaving={isSavingAdvisorConfig}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				isSaveAdvisorConfigError ? (
					<p className="m-0">
						{getErrorMessage(
							saveAdvisorConfigError,
							"Failed to save advisor settings.",
						)}
					</p>
				) : isAdvisorConfigLoadError ? (
					<p className="m-0">Failed to load advisor settings.</p>
				) : undefined
			}
		>
			<div className="flex flex-col gap-6">
				<div className="flex flex-col gap-2">
					<Input
						id={maxUsesId}
						name="max_uses_per_run"
						type="number"
						min={0}
						step={1}
						inputMode="numeric"
						aria-label="Max uses per run"
						value={form.values.max_uses_per_run}
						onChange={(event) =>
							void form.setFieldValue(
								"max_uses_per_run",
								event.currentTarget.value,
							)
						}
						onBlur={form.handleBlur}
						aria-invalid={Boolean(form.errors.max_uses_per_run)}
						disabled={isFormDisabled}
						className="h-10 w-24"
					/>
				</div>

				<div className="flex flex-col gap-2">
					<Input
						id={maxOutputTokensId}
						name="max_output_tokens"
						type="number"
						min={0}
						step={1}
						inputMode="numeric"
						aria-label="Max output tokens"
						value={form.values.max_output_tokens}
						onChange={(event) =>
							void form.setFieldValue(
								"max_output_tokens",
								event.currentTarget.value,
							)
						}
						onBlur={form.handleBlur}
						aria-invalid={Boolean(form.errors.max_output_tokens)}
						disabled={isFormDisabled}
						className="h-10 w-24"
					/>
				</div>

				<div className="flex items-center gap-2">
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
							className="h-10 w-[22rem] max-w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm"
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
					<Button
						size="lg"
						variant="outline"
						type="button"
						onClick={() => {
							void form.setFieldValue("model_config_id", "");
						}}
						disabled={isFormDisabled}
						className="h-10"
					>
						Clear
					</Button>
				</div>
			</div>
		</AgentSettingLayout>
	);
};
