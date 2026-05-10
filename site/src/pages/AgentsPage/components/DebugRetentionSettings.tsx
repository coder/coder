import { useFormik } from "formik";
import type { FC } from "react";
import { useState } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { DefaultChatDebugRetentionDays } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "./TemporarySavedState";

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
const validationSchema = Yup.object({
	debug_retention_days: Yup.number()
		.integer("Debug retention days must be a whole number.")
		.min(1, "Debug retention period must be at least 1 day.")
		.max(3650, "Must not exceed 3650 days (~10 years).")
		.required("Debug retention days is required."),
});

export const DebugRetentionSettings: FC<DebugRetentionSettingsProps> = ({
	debugRetentionDaysData,
	isDebugRetentionDaysLoading,
	isDebugRetentionDaysLoadError,
	onSaveDebugRetentionDays,
	isSavingDebugRetentionDays,
	isSaveDebugRetentionDaysError,
}) => {
	const [debugRetentionToggled, setDebugRetentionToggled] = useState<
		boolean | null
	>(null);
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const serverDebugRetentionDays =
		debugRetentionDaysData?.debug_retention_days ??
		DefaultChatDebugRetentionDays;
	const isDebugRetentionEnabled =
		debugRetentionToggled ?? serverDebugRetentionDays > 0;

	const form = useFormik({
		initialValues: { debug_retention_days: serverDebugRetentionDays },
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveDebugRetentionDays(
				{ debug_retention_days: values.debug_retention_days },
				{
					onSuccess: () => {
						showSavedState();
						setDebugRetentionToggled(null);
						helpers.resetForm();
					},
				},
			);
		},
	});

	const resetDebugRetentionState = () => {
		setDebugRetentionToggled(null);
		form.resetForm();
	};

	const handleToggleDebugRetention = (checked: boolean) => {
		if (checked) {
			const days =
				serverDebugRetentionDays > 0
					? serverDebugRetentionDays
					: DefaultChatDebugRetentionDays;
			setDebugRetentionToggled(true);
			void form.setFieldValue("debug_retention_days", days);
			onSaveDebugRetentionDays(
				{ debug_retention_days: days },
				{
					onSuccess: resetDebugRetentionState,
					onError: resetDebugRetentionState,
				},
			);
		} else {
			setDebugRetentionToggled(false);
			void form.setFieldValue("debug_retention_days", 0);
			onSaveDebugRetentionDays(
				{ debug_retention_days: 0 },
				{
					onSuccess: resetDebugRetentionState,
					onError: resetDebugRetentionState,
				},
			);
		}
	};

	return (
		<form className="flex flex-col gap-2" onSubmit={form.handleSubmit}>
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Chat Debug Data Retention
					</h3>
				</div>
				<Switch
					checked={isDebugRetentionEnabled}
					onCheckedChange={handleToggleDebugRetention}
					aria-label="Enable chat debug data retention"
					disabled={isSavingDebugRetentionDays || isDebugRetentionDaysLoading}
				/>
			</div>
			<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
				Chat debug runs and debug steps older than this are automatically
				deleted. This does not control chat message retention.
			</p>
			{isDebugRetentionEnabled && (
				<>
					<div className="flex gap-2">
						<Input
							type="number"
							name="debug_retention_days"
							min={1}
							max={3650}
							step={1}
							aria-label="Chat debug data retention period in days"
							value={form.values.debug_retention_days}
							onChange={form.handleChange}
							onBlur={form.handleBlur}
							aria-invalid={Boolean(form.errors.debug_retention_days)}
							disabled={
								isSavingDebugRetentionDays || isDebugRetentionDaysLoading
							}
							className="flex-1"
						/>
						<span className="flex h-10 w-[120px] items-center px-3 text-sm text-content-secondary">
							Days
						</span>
					</div>
					{form.errors.debug_retention_days &&
						form.touched.debug_retention_days && (
							<p className="m-0 text-xs text-content-destructive">
								{form.errors.debug_retention_days}
							</p>
						)}
					<div className="mt-2 flex min-h-6 justify-end">
						{(form.dirty || isSavedVisible || isSavingDebugRetentionDays) &&
							(isSavedVisible ? (
								<TemporarySavedState />
							) : (
								<Button
									size="xs"
									type="submit"
									disabled={
										isSavingDebugRetentionDays ||
										!form.dirty ||
										Boolean(form.errors.debug_retention_days)
									}
								>
									{isSavingDebugRetentionDays && (
										<Spinner loading className="h-4 w-4" />
									)}
									Save
								</Button>
							))}
					</div>
				</>
			)}
			{isSaveDebugRetentionDaysError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save chat debug retention setting.
				</p>
			)}
			{isDebugRetentionDaysLoadError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load chat debug retention setting.
				</p>
			)}
		</form>
	);
};
