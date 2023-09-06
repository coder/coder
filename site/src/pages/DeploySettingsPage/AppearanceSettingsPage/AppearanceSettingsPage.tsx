import { UpdateAppearanceConfig } from "api/typesGenerated";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const AppearanceSettingsPage: FC = () => {
  const { appearance, entitlements } = useDashboard();
  const isEntitled =
    entitlements.features["appearance"].entitlement !== "not_entitled";

  const updateAppearance = (
    newConfig: Partial<UpdateAppearanceConfig>,
    preview: boolean,
  ) => {
    const newAppearance = {
      ...appearance.config,
      ...newConfig,
    };
    if (preview) {
      appearance.setPreview(newAppearance);
      return;
    }
    appearance.save(newAppearance);
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Appearance Settings")}</title>
      </Helmet>

      <AppearanceSettingsPageView
        appearance={appearance.config}
        isEntitled={isEntitled}
        updateAppearance={updateAppearance}
      />
    </>
  );
};

export default AppearanceSettingsPage;
