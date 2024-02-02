import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import type { UpdateAppearanceConfig } from "api/typesGenerated";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { updateAppearance } from "api/queries/appearance";
import { getErrorMessage } from "api/errors";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const AppearanceSettingsPage: FC = () => {
  const { appearance, entitlements } = useDashboard();
  const queryClient = useQueryClient();
  const updateAppearanceMutation = useMutation(updateAppearance(queryClient));

  const onSaveAppearance = async (
    newConfig: Partial<UpdateAppearanceConfig>,
    preview: boolean,
  ) => {
    const newAppearance = { ...appearance.config, ...newConfig };
    if (preview) {
      appearance.setPreview(newAppearance);
      return;
    }

    try {
      await updateAppearanceMutation.mutateAsync(newAppearance);
      displaySuccess("Successfully updated appearance settings!");
    } catch (error) {
      displayError(
        getErrorMessage(error, "Failed to update appearance settings."),
      );
    }
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Appearance Settings")}</title>
      </Helmet>

      <AppearanceSettingsPageView
        appearance={appearance.config}
        onSaveAppearance={onSaveAppearance}
        isEntitled={
          entitlements.features.appearance.entitlement !== "not_entitled"
        }
      />
    </>
  );
};

export default AppearanceSettingsPage;
