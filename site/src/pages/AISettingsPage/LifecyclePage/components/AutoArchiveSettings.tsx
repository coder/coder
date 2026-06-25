import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { DefaultChatAutoArchiveDays } from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { DaysField, LifecycleSettingLayout } from "./LifecycleSettingLayout";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AutoArchiveSettingsProps {
	autoArchiveDaysData: TypesGen.ChatAutoArchiveDaysResponse | undefined;
	isAutoArchiveDaysLoading: boolean;
	isAutoArchiveDaysLoadError: boolean;
	onSaveAutoArchiveDays: (
		req: TypesGen.UpdateChatAutoArchiveDaysRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAutoArchiveDays: boolean;
	isSaveAutoArchiveDaysError: boolean;
}

// Keep in sync with autoArchiveDaysMaximum in coderd/exp_chats.go.
const DAYS_MIN = 1;
const DAYS_MAX = 3650;
const ENABLE_DEFAULT_DAYS = 90;

const validationSchema = Yup.object({
	enabled: Yup.boolean().required(),
	auto_archive_days: Yup.number().when("enabled", {
		is: true,
		then: (schema) =>
			schema
				.integer("Auto-archive days must be a whole number.")
				.min(DAYS_MIN, "Auto-archive period must be at least 1 day.")
				.max(DAYS_MAX, "Must not exceed 3650 days (~10 years).")
				.required("Auto-archive days is required."),
	}),
});

export const AutoArchiveSettings: FC<AutoArchiveSettingsProps> = ({
	autoArchiveDaysData,
	isAutoArchiveDaysLoading,
	isAutoArchiveDaysLoadError,
	onSaveAutoArchiveDays,
	isSavingAutoArchiveDays,
	isSaveAutoArchiveDaysError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const serverAutoArchiveDays =
		autoArchiveDaysData?.auto_archive_days ?? DefaultChatAutoArchiveDays;

	const form = useFormik({
		initialValues: {
			enabled: serverAutoArchiveDays > 0,
			auto_archive_days:
				serverAutoArchiveDays > 0 ? serverAutoArchiveDays : ENABLE_DEFAULT_DAYS,
		},
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveAutoArchiveDays(
				{ auto_archive_days: values.enabled ? values.auto_archive_days : 0 },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const fieldError = form.errors.auto_archive_days;
	const hasError =
		(Boolean(fieldError) && Boolean(form.touched.auto_archive_days)) ||
		isSaveAutoArchiveDaysError ||
		isAutoArchiveDaysLoadError;

	return (
		<LifecycleSettingLayout
			title="Auto-archive inactive conversations"
			description="Inactive conversations are automatically archived after this period. Pinned conversations are exempt."
			checked={form.values.enabled}
			onCheckedChange={(checked) => void form.setFieldValue("enabled", checked)}
			switchLabel="Enable auto-archive"
			disabled={isSavingAutoArchiveDays || isAutoArchiveDaysLoading}
			showSave={form.dirty}
			isSaving={isSavingAutoArchiveDays}
			isSavedVisible={isSavedVisible}
			saveDisabled={
				isSavingAutoArchiveDays || !form.dirty || Boolean(fieldError)
			}
			onSubmit={form.handleSubmit}
			error={
				hasError ? (
					<>
						{fieldError && form.touched.auto_archive_days && (
							<p className="m-0">{fieldError}</p>
						)}
						{isSaveAutoArchiveDaysError && (
							<p className="m-0">Failed to save auto-archive setting.</p>
						)}
						{isAutoArchiveDaysLoadError && (
							<p className="m-0">Failed to load auto-archive setting.</p>
						)}
					</>
				) : undefined
			}
		>
			<DaysField
				name="auto_archive_days"
				value={form.values.auto_archive_days}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				label="Auto-archive period in days"
				disabled={
					!form.values.enabled ||
					isSavingAutoArchiveDays ||
					isAutoArchiveDaysLoading
				}
				error={Boolean(fieldError)}
				min={DAYS_MIN}
				max={DAYS_MAX}
			/>
		</LifecycleSettingLayout>
	);
};
