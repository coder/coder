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
	const isDisabled = isSavingAdminSetting || !hasLoadedAdminSettings;
	const allowsUsers = adminSettings?.allow_users ?? false;

	return (
		<div
			role="group"
			aria-label="Allow personal model overrides"
			className="flex flex-col"
		>
			<div className="flex min-h-8 items-start gap-2 font-sans text-sm font-normal leading-6 text-content-primary">
				<Switch
					checked={allowsUsers}
					onCheckedChange={(checked) => {
						onSaveAdminSetting({ allow_users: checked });
					}}
					aria-label="Allow personal model overrides"
					type="button"
					disabled={isDisabled}
					className="mt-0.5"
				/>
				<div className="flex min-w-0 flex-col">
					<span>Allow personal model overrides</span>
					<span className="text-content-secondary">
						Saved user preferences are preserved but ignored while disabled.
					</span>
				</div>
			</div>
			{hasAdminSettingsError && (
				<div className="mt-4 flex flex-col gap-2 text-xs text-content-primary">
					<ErrorAlert error={adminSettingsError} />
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
				<p className="m-0 mt-4 text-xs text-content-secondary">
					Loading personal model override settings...
				</p>
			)}
			{isSaveAdminSettingError && (
				<p className="m-0 mt-4 text-xs text-content-destructive">
					Failed to save personal model override settings.
				</p>
			)}
		</div>
	);
};
