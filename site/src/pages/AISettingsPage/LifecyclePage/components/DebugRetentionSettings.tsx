import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { DefaultChatDebugRetentionDays } from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { DaysField, LifecycleSettingLayout } from "./LifecycleSettingLayout";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface DebugRetentionSettingsProps {
	debugRetentionDaysData: TypesGen.ChatDebugRetentionDaysResponse | undefined;
	isDebugRetentionDaysLoading: boolean;
	isDebugRetentionDaysLoadError: boolean;
	onSaveDebugRetentionDays: (
		req: TypesGen.UpdateChatDebugRetentionDaysRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingDebugRetentionDays: boolean;
	isSaveDebugRetentionDaysError: boolean;
}

// Keep in sync with chatDebugRetentionDaysMaximum in coderd/exp_chats.go.
const DAYS_MIN = 1;
const DAYS_MAX = 3650;

const validationSchema = Yup.object({
	enabled: Yup.boolean().required(),
	debug_retention_days: Yup.number().when("enabled", {
		is: true,
		then: (schema) =>
			schema
				.integer("Debug retention days must be a whole number.")
				.min(DAYS_MIN, "Debug retention period must be at least 1 day.")
				.max(DAYS_MAX, "Must not exceed 3650 days (~10 years).")
				.required("Debug retention days is required."),
	}),
});

export const DebugRetentionSettings: FC<DebugRetentionSettingsProps> = ({
	debugRetentionDaysData,
	isDebugRetentionDaysLoading,
	isDebugRetentionDaysLoadError,
	onSaveDebugRetentionDays,
	isSavingDebugRetentionDays,
	isSaveDebugRetentionDaysError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const serverDebugRetentionDays =
		debugRetentionDaysData?.debug_retention_days ??
		DefaultChatDebugRetentionDays;

	const form = useFormik({
		initialValues: {
			enabled: serverDebugRetentionDays > 0,
			debug_retention_days:
				serverDebugRetentionDays > 0
					? serverDebugRetentionDays
					: DefaultChatDebugRetentionDays,
		},
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveDebugRetentionDays(
				{
					debug_retention_days: values.enabled
						? values.debug_retention_days
						: 0,
				},
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const fieldError = form.errors.debug_retention_days;
	const hasError =
		(Boolean(fieldError) && Boolean(form.touched.debug_retention_days)) ||
		isSaveDebugRetentionDaysError ||
		isDebugRetentionDaysLoadError;

	return (
		<LifecycleSettingLayout
			title="Chat debug data retention"
			description="Chat debug runs and debug steps older than this are automatically deleted. This does not control chat message retention."
			checked={form.values.enabled}
			onCheckedChange={(checked) => void form.setFieldValue("enabled", checked)}
			switchLabel="Enable chat debug data retention"
			disabled={isSavingDebugRetentionDays || isDebugRetentionDaysLoading}
			showSave={form.dirty}
			isSaving={isSavingDebugRetentionDays}
			isSavedVisible={isSavedVisible}
			saveDisabled={
				isSavingDebugRetentionDays || !form.dirty || Boolean(fieldError)
			}
			onSubmit={form.handleSubmit}
			error={
				hasError ? (
					<>
						{fieldError && form.touched.debug_retention_days && (
							<p className="m-0">{fieldError}</p>
						)}
						{isSaveDebugRetentionDaysError && (
							<p className="m-0">
								Failed to save chat debug retention setting.
							</p>
						)}
						{isDebugRetentionDaysLoadError && (
							<p className="m-0">
								Failed to load chat debug retention setting.
							</p>
						)}
					</>
				) : undefined
			}
		>
			<DaysField
				name="debug_retention_days"
				value={form.values.debug_retention_days}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				label="Chat debug data retention period in days"
				disabled={
					!form.values.enabled ||
					isSavingDebugRetentionDays ||
					isDebugRetentionDaysLoading
				}
				error={Boolean(fieldError)}
				min={DAYS_MIN}
				max={DAYS_MAX}
			/>
		</LifecycleSettingLayout>
	);
};
