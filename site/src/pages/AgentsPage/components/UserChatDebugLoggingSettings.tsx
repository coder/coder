import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";

interface UserChatDebugLoggingSettingsProps {
	userSettings: TypesGen.UserChatDebugLoggingSettings | undefined;
	onSaveUserSetting: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateUserChatDebugLoggingRequest,
		unknown
	>;
	isSavingUserSetting: boolean;
	isSaveUserSettingError: boolean;
}

export const UserChatDebugLoggingSettings: FC<
	UserChatDebugLoggingSettingsProps
> = ({
	userSettings,
	onSaveUserSetting,
	isSavingUserSetting,
	isSaveUserSettingError,
}) => {
	if (!userSettings?.user_toggle_allowed) {
		return null;
	}

	const forcedByDeployment = userSettings.forced_by_deployment;
	const userDebugLoggingEnabled = userSettings.debug_logging_enabled;

	return (
		<div className="space-y-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Record debug logs for my chats
			</h3>
			<div className="flex items-center justify-between gap-4">
				<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
					{forcedByDeployment ? (
						<p className="m-0">
							An administrator has enabled debug logging for every chat in this
							deployment, so this toggle is locked on.
						</p>
					) : (
						<p className="m-0">
							Save a detailed trace of your chats: each turn plus the raw API
							requests and responses sent to the model provider. Useful for
							troubleshooting unexpected model behavior.
						</p>
					)}
				</div>
				<Switch
					checked={forcedByDeployment || userDebugLoggingEnabled}
					onCheckedChange={(checked) =>
						onSaveUserSetting({ debug_logging_enabled: checked })
					}
					aria-label="Enable personal chat debug logging"
					disabled={forcedByDeployment || isSavingUserSetting}
				/>
			</div>
			{isSaveUserSettingError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save your chat debug logging preference.
				</p>
			)}
		</div>
	);
};
