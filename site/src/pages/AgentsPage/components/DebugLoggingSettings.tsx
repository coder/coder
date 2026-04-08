import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { AdminBadge } from "./AdminBadge";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface DebugLoggingSettingsProps {
	canManageAdminSetting: boolean;
	adminSettings: TypesGen.ChatDebugLoggingAdminSettings | undefined;
	userSettings: TypesGen.UserChatDebugLoggingSettings | undefined;
	onSaveAdminSetting: (
		req: TypesGen.UpdateChatDebugLoggingAllowUsersRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdminSetting: boolean;
	isSaveAdminSettingError: boolean;
	onSaveUserSetting: (
		req: TypesGen.UpdateUserChatDebugLoggingRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingUserSetting: boolean;
	isSaveUserSettingError: boolean;
}

export const DebugLoggingSettings: FC<DebugLoggingSettingsProps> = ({
	canManageAdminSetting,
	adminSettings,
	userSettings,
	onSaveAdminSetting,
	isSavingAdminSetting,
	isSaveAdminSettingError,
	onSaveUserSetting,
	isSavingUserSetting,
	isSaveUserSettingError,
}) => {
	const forcedByDeployment =
		userSettings?.forced_by_deployment ??
		adminSettings?.forced_by_deployment ??
		false;
	const adminAllowsUsers = adminSettings?.allow_users ?? false;
	const userDebugLoggingEnabled = userSettings?.debug_logging_enabled ?? false;
	const userToggleAllowed = userSettings?.user_toggle_allowed ?? false;

	return (
		<div className="space-y-4">
			{canManageAdminSetting && (
				<div className="space-y-2">
					<div className="flex items-center gap-2">
						<h3 className="m-0 text-[13px] font-semibold text-content-primary">
							Allow User Debug Logs
						</h3>
						<AdminBadge />
					</div>
					<div className="flex items-center justify-between gap-4">
						<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
							{forcedByDeployment ? (
								<p className="m-0">
									Deployment configuration already forces chat debug logging on
									for every chat. This runtime user opt-in setting is currently
									ignored.
								</p>
							) : (
								<p className="m-0">
									Allow users to opt into normalized model state and raw
									provider request/response logging from their personal Behavior
									settings.
								</p>
							)}
						</div>
						<Switch
							checked={adminAllowsUsers}
							onCheckedChange={(checked) =>
								onSaveAdminSetting({ allow_users: checked })
							}
							aria-label="Allow users to enable chat debug logging"
							disabled={forcedByDeployment || isSavingAdminSetting}
						/>
					</div>
					{isSaveAdminSettingError && (
						<p className="m-0 text-xs text-content-destructive">
							Failed to save the admin debug logging setting.
						</p>
					)}
				</div>
			)}

			<div className="space-y-2">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-[13px] font-semibold text-content-primary">
						Personal Chat Debug Logs
					</h3>
				</div>
				<div className="flex items-center justify-between gap-4">
					<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
						{forcedByDeployment ? (
							<p className="m-0">
								Deployment configuration forces chat debug logging on for every
								chat. Your personal toggle is read-only while this is enabled.
							</p>
						) : userToggleAllowed ? (
							<p className="m-0">
								Capture normalized model state and raw provider request/response
								payloads for your own chats.
							</p>
						) : (
							<p className="m-0">
								An administrator has not enabled user-controlled chat debug
								logging yet.
							</p>
						)}
					</div>
					<Switch
						checked={userDebugLoggingEnabled}
						onCheckedChange={(checked) =>
							onSaveUserSetting({ debug_logging_enabled: checked })
						}
						aria-label="Enable personal chat debug logging"
						disabled={
							forcedByDeployment || !userToggleAllowed || isSavingUserSetting
						}
					/>
				</div>
				{isSaveUserSettingError && (
					<p className="m-0 text-xs text-content-destructive">
						Failed to save your chat debug logging preference.
					</p>
				)}
			</div>
		</div>
	);
};
