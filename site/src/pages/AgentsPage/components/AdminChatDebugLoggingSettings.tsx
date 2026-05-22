import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Switch } from "#/components/Switch/Switch";

interface AdminChatDebugLoggingSettingsProps {
	adminSettings: TypesGen.ChatDebugLoggingAdminSettings | undefined;
	isLoadingAdminSetting: boolean;
	onSaveAdminSetting: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatDebugLoggingAllowUsersRequest,
		unknown
	>;
	isSavingAdminSetting: boolean;
	isSaveAdminSettingError: boolean;
}

export const AdminChatDebugLoggingSettings: FC<
	AdminChatDebugLoggingSettingsProps
> = ({
	adminSettings,
	isLoadingAdminSetting,
	onSaveAdminSetting,
	isSavingAdminSetting,
	isSaveAdminSettingError,
}) => {
	const forcedByDeployment = adminSettings?.forced_by_deployment ?? false;
	const adminAllowsUsers = adminSettings?.allow_users ?? false;

	return (
		<div className="space-y-2">
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-sm font-semibold text-content-primary">
					Let users record chat debug logs
				</h3>
			</div>
			<div className="flex items-center justify-between gap-4">
				<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
					{forcedByDeployment ? (
						<p className="m-0">
							Debug logging is already enabled deployment-wide, so this per-user
							setting has no effect right now.
						</p>
					) : (
						<p className="m-0">
							Lets users turn on debug logging for their own chats from their
							General settings. When on, Coder saves each chat turn along with
							the raw API requests and responses sent to the model provider.
						</p>
					)}
				</div>
				<div className="flex items-center gap-2">
					{isLoadingAdminSetting ? (
						<Skeleton className="h-5 w-10 rounded-full" aria-hidden="true" />
					) : (
						<Switch
							checked={adminAllowsUsers}
							onCheckedChange={(checked) =>
								onSaveAdminSetting({ allow_users: checked })
							}
							aria-label="Allow users to enable chat debug logging"
							disabled={
								forcedByDeployment ||
								isSavingAdminSetting ||
								isLoadingAdminSetting
							}
						/>
					)}
				</div>
			</div>
			{isSaveAdminSettingError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save the admin debug logging setting.
				</p>
			)}
		</div>
	);
};
