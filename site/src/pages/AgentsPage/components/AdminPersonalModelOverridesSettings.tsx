import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Switch } from "#/components/Switch/Switch";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

export type SavePersonalModelOverridesAdminSetting = (
	req: TypesGen.UpdateChatPersonalModelOverridesAdminSettingsRequest,
	options?: MutationCallbacks,
) => void;

interface AdminPersonalModelOverridesSettingsProps {
	adminSettings: TypesGen.ChatPersonalModelOverridesAdminSettings | undefined;
	adminSettingsError?: unknown;
	onRetryAdminSettings?: () => void;
	isRetryingAdminSettings?: boolean;
	onSaveAdminSetting: SavePersonalModelOverridesAdminSetting;
	isSavingAdminSetting: boolean;
	isSaveAdminSettingError: boolean;
}

export const AdminPersonalModelOverridesSettings: FC<
	AdminPersonalModelOverridesSettingsProps
> = ({
	adminSettings,
	adminSettingsError,
	onRetryAdminSettings,
	isRetryingAdminSettings = false,
	onSaveAdminSetting,
	isSavingAdminSetting,
	isSaveAdminSettingError,
}) => {
	const hasLoadedAdminSettings = adminSettings !== undefined;
	const hasAdminSettingsError = adminSettingsError != null;
	const form = useFormik({
		enableReinitialize: true,
		initialValues: {
			allow_users: adminSettings?.allow_users ?? false,
		},
		onSubmit: (values, { resetForm }) => {
			onSaveAdminSetting(
				{
					allow_users: values.allow_users,
				},
				{
					onSuccess: () => {
						resetForm({ values });
					},
				},
			);
		},
	});
	const isDisabled = isSavingAdminSetting || !hasLoadedAdminSettings;

	return (
		<form
			aria-label="Personal model overrides"
			className="space-y-2"
			onSubmit={form.handleSubmit}
		>
			<div className="flex items-center justify-between gap-4">
				<div className="space-y-1">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Enable users to define their personal overrides
					</h3>
					<p className="m-0 text-xs text-content-secondary">
						Lets users choose personal models for root chats, General subagents,
						and Explore subagents. When disabled, saved user settings remain
						stored but are ignored at runtime.
					</p>
				</div>
				<Switch
					checked={form.values.allow_users}
					onCheckedChange={(checked) => {
						void form.setFieldValue("allow_users", checked);
					}}
					aria-label="Enable users to define their personal overrides"
					type="button"
					disabled={isDisabled}
				/>
			</div>
			{hasAdminSettingsError ? (
				<div className="flex flex-col gap-2">
					<ErrorAlert error={adminSettingsError} />
					{onRetryAdminSettings && (
						<Button
							disabled={isRetryingAdminSettings}
							onClick={onRetryAdminSettings}
							size="sm"
							type="button"
							variant="outline"
						>
							Retry
						</Button>
					)}
				</div>
			) : (
				!hasLoadedAdminSettings && (
					<p className="m-0 text-xs text-content-secondary">
						Loading personal model override settings...
					</p>
				)
			)}
			<div className="flex justify-end gap-2">
				<Button size="sm" type="submit" disabled={isDisabled || !form.dirty}>
					Save
				</Button>
			</div>
			{isSaveAdminSettingError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save personal model override settings.
				</p>
			)}
		</form>
	);
};
