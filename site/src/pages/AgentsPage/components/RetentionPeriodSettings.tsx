import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { DaysField, LifecycleSettingLayout } from "./LifecycleSettingLayout";
import { useTemporarySavedState } from "./TemporarySavedState";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface RetentionPeriodSettingsProps {
	retentionDaysData: TypesGen.ChatRetentionDaysResponse | undefined;
	isRetentionDaysLoading: boolean;
	isRetentionDaysLoadError: boolean;
	onSaveRetentionDays: (
		req: TypesGen.UpdateChatRetentionDaysRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingRetentionDays: boolean;
	isSaveRetentionDaysError: boolean;
}

// Keep in sync with retentionDaysMaximum in coderd/exp_chats.go.
const validationSchema = Yup.object({
	enabled: Yup.boolean().required(),
	retention_days: Yup.number().when("enabled", {
		is: true,
		then: (schema) =>
			schema
				.integer("Retention days must be a whole number.")
				.min(1, "Retention period must be at least 1 day.")
				.max(3650, "Must not exceed 3650 days (~10 years).")
				.required("Retention days is required."),
	}),
});

export const RetentionPeriodSettings: FC<RetentionPeriodSettingsProps> = ({
	retentionDaysData,
	isRetentionDaysLoading,
	isRetentionDaysLoadError,
	onSaveRetentionDays,
	isSavingRetentionDays,
	isSaveRetentionDaysError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const serverRetentionDays = retentionDaysData?.retention_days ?? 30;

	const form = useFormik({
		initialValues: {
			enabled: serverRetentionDays > 0,
			retention_days: serverRetentionDays > 0 ? serverRetentionDays : 30,
		},
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveRetentionDays(
				{ retention_days: values.enabled ? values.retention_days : 0 },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const fieldError = form.errors.retention_days;
	const hasError =
		(Boolean(fieldError) && Boolean(form.touched.retention_days)) ||
		isSaveRetentionDaysError ||
		isRetentionDaysLoadError;

	return (
		<LifecycleSettingLayout
			title="Conversation retention period"
			description="Archived conversations and orphaned files older than this are automatically deleted."
			checked={form.values.enabled}
			onCheckedChange={(checked) => void form.setFieldValue("enabled", checked)}
			switchLabel="Enable conversation retention"
			disabled={isSavingRetentionDays || isRetentionDaysLoading}
			showSave={form.dirty}
			isSaving={isSavingRetentionDays}
			isSavedVisible={isSavedVisible}
			saveDisabled={isSavingRetentionDays || !form.dirty || Boolean(fieldError)}
			onSubmit={form.handleSubmit}
			error={
				hasError ? (
					<>
						{fieldError && form.touched.retention_days && (
							<p className="m-0">{fieldError}</p>
						)}
						{isSaveRetentionDaysError && (
							<p className="m-0">Failed to save retention setting.</p>
						)}
						{isRetentionDaysLoadError && (
							<p className="m-0">Failed to load retention setting.</p>
						)}
					</>
				) : undefined
			}
		>
			<DaysField
				name="retention_days"
				value={form.values.retention_days}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				label="Conversation retention period in days"
				disabled={isSavingRetentionDays || isRetentionDaysLoading}
				error={Boolean(fieldError)}
			/>
		</LifecycleSettingLayout>
	);
};
