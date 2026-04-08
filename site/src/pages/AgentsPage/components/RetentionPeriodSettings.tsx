import type { FC, FormEvent } from "react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Switch } from "#/components/Switch/Switch";
import { AdminBadge } from "./AdminBadge";

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

export const RetentionPeriodSettings: FC<RetentionPeriodSettingsProps> = ({
	retentionDaysData,
	isRetentionDaysLoading,
	isRetentionDaysLoadError,
	onSaveRetentionDays,
	isSavingRetentionDays,
	isSaveRetentionDaysError,
}) => {
	const [localRetentionDays, setLocalRetentionDays] = useState<number | null>(
		null,
	);
	const [retentionToggled, setRetentionToggled] = useState<boolean | null>(
		null,
	);

	const serverRetentionDays = retentionDaysData?.retention_days ?? 30;
	const retentionDays = localRetentionDays ?? serverRetentionDays;
	const isRetentionEnabled = retentionToggled ?? serverRetentionDays > 0;
	const isRetentionDaysDirty =
		localRetentionDays !== null && localRetentionDays !== serverRetentionDays;
	const isRetentionDaysNegative = isRetentionEnabled && retentionDays < 0;
	// Keep in sync with retentionDaysMaximum in coderd/exp_chats.go.
	const retentionDaysMaximum = 3650;
	const isRetentionDaysOverMax = retentionDays > retentionDaysMaximum;
	const isRetentionDaysZero = isRetentionEnabled && retentionDays === 0;

	const resetRetentionState = () => {
		setLocalRetentionDays(null);
		setRetentionToggled(null);
	};

	const handleToggleRetention = (checked: boolean) => {
		if (checked) {
			setRetentionToggled(true);
			setLocalRetentionDays(serverRetentionDays > 0 ? serverRetentionDays : 30);
			onSaveRetentionDays(
				{
					retention_days: serverRetentionDays > 0 ? serverRetentionDays : 30,
				},
				{
					onSuccess: resetRetentionState,
					onError: resetRetentionState,
				},
			);
		} else {
			setRetentionToggled(false);
			setLocalRetentionDays(0);
			onSaveRetentionDays(
				{ retention_days: 0 },
				{
					onSuccess: resetRetentionState,
					onError: resetRetentionState,
				},
			);
		}
	};

	const handleRetentionDaysChange = (value: number) => {
		setLocalRetentionDays(value);
		if (retentionToggled === null) {
			setRetentionToggled(true);
		}
	};

	const handleSaveRetentionDays = (event: FormEvent) => {
		event.preventDefault();
		if (!isRetentionDaysDirty || isSavingRetentionDays) return;
		onSaveRetentionDays(
			{ retention_days: localRetentionDays ?? 30 },
			{ onSuccess: resetRetentionState },
		);
	};

	return (
		<form
			className="space-y-2"
			onSubmit={(event) => void handleSaveRetentionDays(event)}
		>
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Conversation Retention Period
				</h3>
				<AdminBadge />
			</div>
			<div className="flex items-center justify-between gap-4">
				<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
					Archived conversations and orphaned files older than this are
					automatically deleted.
				</p>
				<Switch
					checked={isRetentionEnabled}
					onCheckedChange={handleToggleRetention}
					aria-label="Enable conversation retention"
					disabled={isSavingRetentionDays || isRetentionDaysLoading}
				/>
			</div>
			{isRetentionEnabled && (
				<>
					<input
						type="number"
						min={1}
						max={3650}
						step={1}
						aria-label="Conversation retention period in days"
						value={retentionDays}
						onChange={(event) =>
							handleRetentionDaysChange(
								Number.parseInt(event.target.value, 10) || 0,
							)
						}
						disabled={isSavingRetentionDays || isRetentionDaysLoading}
						className="w-full rounded-lg border border-border bg-surface-primary px-4 py-2 text-[13px] text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-link/30"
					/>
					{isRetentionDaysZero && (
						<p className="m-0 text-xs text-content-destructive">
							Retention period must be at least 1 day.
						</p>
					)}
					{isRetentionDaysNegative && (
						<p className="m-0 text-xs text-content-destructive">
							Retention days cannot be negative.
						</p>
					)}
					{isRetentionDaysOverMax && (
						<p className="m-0 text-xs text-content-destructive">
							Must not exceed {retentionDaysMaximum} days (~10 years).
						</p>
					)}
					<div className="flex justify-end">
						<Button
							size="sm"
							type="submit"
							disabled={
								isSavingRetentionDays ||
								!isRetentionDaysDirty ||
								isRetentionDaysNegative ||
								isRetentionDaysOverMax ||
								isRetentionDaysZero
							}
						>
							Save
						</Button>
					</div>
				</>
			)}
			{isSaveRetentionDaysError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save retention setting.
				</p>
			)}
			{isRetentionDaysLoadError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load retention setting.
				</p>
			)}
		</form>
	);
};
