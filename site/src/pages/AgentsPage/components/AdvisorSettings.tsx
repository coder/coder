import { useFormik } from "formik";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type {
	AdvisorConfig,
	UpdateAdvisorConfigRequest,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { AgentSettingLayout } from "#/pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout";
import { cn } from "#/utils/cn";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AdvisorSettingsProps {
	advisorConfigData: AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
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
};

const normalizeNonNegativeInteger = (
	value: number | string | undefined,
): number => {
	const parsed = typeof value === "number" ? value : Number(value);
	return Number.isFinite(parsed) && parsed >= 0 ? Math.trunc(parsed) : 0;
};

const normalizeAdvisorConfig = (
	config: AdvisorConfig | UpdateAdvisorConfigRequest | undefined,
): AdvisorSettingsFormValues => ({
	max_uses_per_run: String(
		normalizeNonNegativeInteger(config?.max_uses_per_run),
	),
	max_output_tokens: String(
		normalizeNonNegativeInteger(config?.max_output_tokens),
	),
});

const isNonNegativeIntegerString = (value: string): boolean => {
	const parsed = Number(value);
	return (
		value.trim() !== "" &&
		Number.isFinite(parsed) &&
		parsed >= 0 &&
		Number.isInteger(parsed)
	);
};

const validateAdvisorConfig = (values: AdvisorSettingsFormValues) => {
	const errors: Partial<Record<keyof AdvisorSettingsFormValues, string>> = {};
	if (!isNonNegativeIntegerString(values.max_uses_per_run))
		errors.max_uses_per_run =
			"Max uses per turn must be a non-negative integer.";
	if (!isNonNegativeIntegerString(values.max_output_tokens))
		errors.max_output_tokens =
			"Max output tokens must be a non-negative integer.";
	return errors;
};

export const AdvisorSettings: FC<AdvisorSettingsProps> = ({
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
}) => {
	const maxUsesId = useId();
	const maxOutputTokensId = useId();
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const hasLoadedAdvisorConfig = advisorConfigData !== undefined;
	const form = useFormik<AdvisorSettingsFormValues>({
		enableReinitialize: true,
		validateOnMount: true,
		initialValues: normalizeAdvisorConfig(advisorConfigData),
		validate: validateAdvisorConfig,
		onSubmit: (values, { resetForm }) => {
			const request: UpdateAdvisorConfigRequest = {
				max_uses_per_run: normalizeNonNegativeInteger(values.max_uses_per_run),
				max_output_tokens: normalizeNonNegativeInteger(
					values.max_output_tokens,
				),
			};
			onSaveAdvisorConfig(request, {
				onSuccess: () => {
					showSavedState();
					resetForm({ values: normalizeAdvisorConfig(request) });
				},
			});
		},
	});
	const isFormDisabled =
		isSavingAdvisorConfig ||
		isAdvisorConfigLoading ||
		isAdvisorConfigFetching ||
		!hasLoadedAdvisorConfig;
	const canSave = hasLoadedAdvisorConfig && form.dirty && form.isValid;
	return (
		<AgentSettingLayout
			title="Advisor"
			description="Cap advisor usage per turn. Configure its model in Organization settings above. Set limits to 0 for unlimited."
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
			<CompactIntegerField
				id={maxUsesId}
				name="max_uses_per_run"
				label="Uses / turn"
				ariaLabel="Uses / turn"
				value={form.values.max_uses_per_run}
				onChange={(value) => void form.setFieldValue("max_uses_per_run", value)}
				onBlur={form.handleBlur}
				error={Boolean(form.errors.max_uses_per_run)}
				disabled={isFormDisabled}
				className="w-[7.5rem]"
			/>
			<CompactIntegerField
				id={maxOutputTokensId}
				name="max_output_tokens"
				label="Max tokens"
				ariaLabel="Max tokens"
				value={form.values.max_output_tokens}
				onChange={(value) =>
					void form.setFieldValue("max_output_tokens", value)
				}
				onBlur={form.handleBlur}
				error={Boolean(form.errors.max_output_tokens)}
				disabled={isFormDisabled}
				className="w-36"
			/>
			<Button
				size="lg"
				variant="outline"
				type="button"
				onClick={() =>
					void form.setValues({ max_uses_per_run: "0", max_output_tokens: "0" })
				}
				disabled={isFormDisabled}
				className="h-10"
			>
				Clear
			</Button>
		</AgentSettingLayout>
	);
};

interface CompactIntegerFieldProps {
	id: string;
	name: string;
	label: string;
	ariaLabel: string;
	value: string;
	onChange: (value: string) => void;
	onBlur: (event: React.FocusEvent<HTMLInputElement>) => void;
	error?: boolean;
	disabled?: boolean;
	className?: string;
}

const CompactIntegerField: FC<CompactIntegerFieldProps> = ({
	id,
	name,
	label,
	ariaLabel,
	value,
	onChange,
	onBlur,
	error,
	disabled,
	className,
}) => {
	return (
		<label
			className={cn(
				"grid h-10 shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-md border border-border border-solid bg-transparent px-3 transition-colors",
				error && "border-border-destructive",
				disabled && "opacity-50",
				className,
			)}
		>
			<input
				id={id}
				type="number"
				name={name}
				min={0}
				step={1}
				inputMode="numeric"
				aria-label={ariaLabel}
				value={value}
				onChange={(event) => onChange(event.currentTarget.value)}
				onBlur={onBlur}
				aria-invalid={error}
				disabled={disabled}
				className="min-w-0 w-full border-none bg-transparent p-0 text-sm font-medium leading-6 text-content-placeholder outline-none disabled:cursor-not-allowed [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none [-moz-appearance:textfield]"
			/>
			<span className="shrink-0 text-xs font-normal leading-[18px] text-content-secondary">
				{label}
			</span>
		</label>
	);
};
