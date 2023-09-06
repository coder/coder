import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { GitAuthSettingsPageView } from "./GitAuthSettingsPageView";

const GitAuthSettingsPage: FC = () => {
  const { deploymentValues: deploymentValues } = useDeploySettings();

  return (
    <>
      <Helmet>
        <title>{pageTitle("Git Authentication Settings")}</title>
      </Helmet>

      <GitAuthSettingsPageView config={deploymentValues.config} />
    </>
  );
};

export default GitAuthSettingsPage;
