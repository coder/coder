import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const SecuritySettingsPage: FC = () => {
  const { deploymentValues: deploymentValues } = useDeploySettings();
  const { entitlements } = useDashboard();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Security Settings")}</title>
      </Helmet>

      <SecuritySettingsPageView
        options={deploymentValues.options}
        featureBrowserOnlyEnabled={
          entitlements.features["browser_only"].enabled
        }
      />
    </>
  );
};

export default SecuritySettingsPage;
