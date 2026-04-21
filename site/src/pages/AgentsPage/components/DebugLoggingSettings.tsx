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
							Let users record chat debug logs
						</h3>
						<AdminBadge />
					</div>
					<div className="flex items-center justify-between gap-4">
						<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
							{forcedByDeployment ? (
								<p className="m-0">
									Debug logging is already enabled deployment-wide, so this
									per-user setting has no effect right now.
								</p>
							) : (
								<p className="m-0">
									Lets users turn on debug logging for their own chats from
									their Behavior settings. When on, Coder saves each chat turn
									along with the raw API requests and responses sent to the
									model provider.
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
						Record debug logs for my chats
					</h3>
				</div>
				<div className="flex items-center justify-between gap-4">
					<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
						{forcedByDeployment ? (
							<p className="m-0">
								An administrator has enabled debug logging for every chat in
								this deployment, so this toggle is locked on.
							</p>
						) : userToggleAllowed ? (
							<p className="m-0">
								Save a detailed trace of your chats: each turn plus the raw API
								requests and responses sent to the model provider. Useful for
								troubleshooting unexpected model behavior.
							</p>
						) : (
							<p className="m-0">
								An administrator hasn't allowed users to record chat debug logs
								yet.
							</p>
						)}
					</div>
					<Switch
						checked={forcedByDeployment || userDebugLoggingEnabled}
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
