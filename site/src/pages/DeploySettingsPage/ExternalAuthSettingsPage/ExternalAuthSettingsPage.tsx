import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
  const { deploymentValues: deploymentValues } = useDeploySettings();

  return (
    <>
      <Helmet>
        <title>{pageTitle("External Authentication Settings")}</title>
      </Helmet>

      <ExternalAuthSettingsPageView config={deploymentValues.config} />
    </>
  );
};

export default ExternalAuthSettingsPage;
