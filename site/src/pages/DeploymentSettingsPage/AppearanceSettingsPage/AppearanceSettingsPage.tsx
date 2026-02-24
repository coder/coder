import { getErrorDetail, getErrorMessage } from "api/errors";
import { appearanceConfigKey, updateAppearance } from "api/queries/appearance";
import type { UpdateAppearanceConfig } from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const AppearanceSettingsPage: FC = () => {
	const { appearance, entitlements } = useDashboard();
	const { multiple_organizations: hasPremiumLicense } = useFeatureVisibility();
	const queryClient = useQueryClient();
	const updateAppearanceMutation = useMutation(updateAppearance(queryClient));

	const onSaveAppearance = async (
		newConfig: Partial<UpdateAppearanceConfig>,
	) => {
		const newAppearance = { ...appearance, ...newConfig };

		const mutation = updateAppearanceMutation.mutateAsync(newAppearance, {
			onSuccess: async () => {
				await queryClient.invalidateQueries({ queryKey: appearanceConfigKey });
			},
		});

		toast.promise(mutation, {
			loading: "Updating appearance settings...",
			success: "Appearance settings updated successfully.",
			error: (error) => ({
				message: getErrorMessage(
					error,
					"Failed to update appearance settings.",
				),
				description: getErrorDetail(error),
			}),
		});
	};

	return (
		<>
			<title>{pageTitle("Appearance Settings")}</title>

			<AppearanceSettingsPageView
				appearance={appearance}
				onSaveAppearance={onSaveAppearance}
				isEntitled={
					entitlements.features.appearance.entitlement !== "not_entitled"
				}
				isPremium={hasPremiumLicense}
			/>
		</>
	);
};

export default AppearanceSettingsPage;
