import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const NetworkSettingsPage: FC = () => {
  const { deploymentValues: deploymentValues } = useDeploySettings();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Network Settings")}</title>
      </Helmet>

      <NetworkSettingsPageView options={deploymentValues.options} />
    </>
  );
};

export default NetworkSettingsPage;
