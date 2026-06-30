import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { LifecycleSettingLayout } from "#/pages/AISettingsPage/LifecyclePage/components/LifecycleSettingLayout";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AdminChatDebugLoggingSettingsProps {
	adminSettings: TypesGen.ChatDebugLoggingAdminSettings | undefined;
	isLoadingAdminSetting: boolean;
	onSaveAdminSetting: (
		req: TypesGen.UpdateChatDebugLoggingAllowUsersRequest,
		options?: MutationCallbacks,
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
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const forcedByDeployment = adminSettings?.forced_by_deployment ?? false;
	const serverAllowsUsers = adminSettings?.allow_users ?? false;
	const hasLoaded = adminSettings !== undefined;

	const form = useFormik({
		enableReinitialize: true,
		initialValues: {
			allow_users: serverAllowsUsers,
		},
		onSubmit: (values, helpers) => {
			onSaveAdminSetting(
				{ allow_users: values.allow_users },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const description = forcedByDeployment
		? "Debug logging is already enabled deployment-wide, so this per-user setting has no effect right now."
		: "Lets users turn on debug logging for their own chats from their General settings. When on, Coder saves each chat turn along with the raw API requests and responses sent to the model provider.";

	return (
		<LifecycleSettingLayout
			title="Let users record chat debug logs"
			description={description}
			checked={form.values.allow_users}
			onCheckedChange={(checked) =>
				void form.setFieldValue("allow_users", checked)
			}
			switchLabel="Allow users to enable chat debug logging"
			disabled={
				forcedByDeployment || isSavingAdminSetting || isLoadingAdminSetting
			}
			showSave={form.dirty}
			isSaving={isSavingAdminSetting}
			isSavedVisible={isSavedVisible}
			saveDisabled={
				isSavingAdminSetting || !form.dirty || !hasLoaded || forcedByDeployment
			}
			onSubmit={form.handleSubmit}
			error={
				isSaveAdminSettingError ? (
					<p className="m-0">Failed to save the admin debug logging setting.</p>
				) : undefined
			}
		/>
	);
};
