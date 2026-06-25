import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/components/TemporarySavedState/TemporarySavedState";

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
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
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
						showSavedState();
						resetForm({ values });
					},
				},
			);
		},
	});
	const isDisabled = isSavingAdminSetting || !hasLoadedAdminSettings;
	const showSave = form.dirty || isSavingAdminSetting || isSavedVisible;
	const hasError =
		hasAdminSettingsError || !hasLoadedAdminSettings || isSaveAdminSettingError;

	return (
		<form className="flex flex-col" onSubmit={form.handleSubmit} noValidate>
			<div className="flex min-h-8 items-center gap-2 font-sans text-sm font-normal leading-6 text-content-primary">
				<Switch
					checked={form.values.allow_users}
					onCheckedChange={(checked) => {
						void form.setFieldValue("allow_users", checked);
					}}
					aria-label="Allow personal model overrides"
					type="button"
					disabled={isDisabled}
				/>
				<div className="flex min-w-0 items-center gap-1.5">
					<span>Allow personal model overrides</span>
				</div>
			</div>
			{showSave && (
				<div className="mt-4 flex min-h-10 items-center">
					{isSavedVisible ? (
						<TemporarySavedState />
					) : (
						<Button
							size="lg"
							type="submit"
							disabled={isDisabled || !form.dirty}
							className="h-10 min-w-[88px]"
						>
							{isSavingAdminSetting && <Spinner loading className="h-4 w-4" />}
							Save
						</Button>
					)}
				</div>
			)}
			{hasError && (
				<div className="text-xs text-content-destructive">
					{hasAdminSettingsError && (
						<div className="flex flex-col gap-2 text-content-primary">
							<p className="m-0 text-content-destructive">
								Failed to load personal model override settings.
							</p>
							{onRetryAdminSettings && (
								<Button
									disabled={isRetryingAdminSettings}
									onClick={onRetryAdminSettings}
									size="sm"
									type="button"
									variant="outline"
									className="w-fit"
								>
									Retry
								</Button>
							)}
						</div>
					)}
					{!hasAdminSettingsError && !hasLoadedAdminSettings && (
						<p className="m-0 text-content-secondary">
							Loading personal model override settings...
						</p>
					)}
					{isSaveAdminSettingError && (
						<p className="m-0">
							Failed to save personal model override settings.
						</p>
					)}
				</div>
			)}
		</form>
	);
};
