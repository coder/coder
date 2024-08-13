import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { ExternalAuthSettingsPageView } from "./ExternalAuthSettingsPageView";

const ExternalAuthSettingsPage: FC = () => {
  const { deploymentValues } = useDeploySettings();

  return (
    <>
      <Helmet>
        <title>{pageTitle("External Authentication Settings")}</title>
      </Helmet>

      {deploymentValues ? (
        <ExternalAuthSettingsPageView config={deploymentValues.config} />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default ExternalAuthSettingsPage;
