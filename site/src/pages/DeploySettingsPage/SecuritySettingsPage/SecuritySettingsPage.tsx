import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useDashboard } from "modules/dashboard/useDashboard";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";
import { useDeploySettings } from "../DeploySettingsLayout";

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
