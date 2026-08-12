import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Switch } from "#/components/Switch/Switch";
import { cn } from "#/utils/cn";

interface AdminChatDebugLoggingSettingsProps {
	adminSettings: TypesGen.ChatDebugLoggingAdminSettings | undefined;
	isLoadingAdminSetting: boolean;
	onSaveAdminSetting: (
		req: TypesGen.UpdateChatDebugLoggingAllowUsersRequest,
	) => void;
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

	const description = forcedByDeployment
		? "Debug logging is already enabled deployment-wide, so this per-user setting has no effect right now."
		: "Lets users turn on debug logging for their own chats from their General settings. When on, Coder saves each chat turn along with the raw API requests and responses sent to the model provider.";

	return (
		<div className="flex items-start gap-3">
			<Switch
				checked={adminAllowsUsers}
				onCheckedChange={(checked) =>
					onSaveAdminSetting({ allow_users: checked })
				}
				aria-label="Allow users to enable chat debug logging"
				disabled={
					forcedByDeployment || isSavingAdminSetting || isLoadingAdminSetting
				}
				className={cn("mt-0.5")}
			/>
			<div className="flex max-w-[980px] flex-1 flex-col">
				<h3 className="m-0 text-sm font-normal leading-6 text-content-primary">
					Let users record chat debug logs
				</h3>
				<p className="mt-1 mb-0 text-sm font-normal leading-6 text-content-secondary">
					{description}
				</p>
				{isSaveAdminSettingError && (
					<p className="m-0 mt-2 text-xs text-content-destructive">
						Failed to save the admin debug logging setting.
					</p>
				)}
			</div>
		</div>
	);
};
