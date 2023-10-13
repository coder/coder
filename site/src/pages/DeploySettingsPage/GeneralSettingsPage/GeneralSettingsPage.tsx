import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { GeneralSettingsPageView } from "./GeneralSettingsPageView";
import { useQuery } from "react-query";
import { deploymentDAUs } from "api/queries/deployment";
import { entitlements } from "api/queries/entitlements";

const GeneralSettingsPage: FC = () => {
  const { deploymentValues } = useDeploySettings();
  const deploymentDAUsQuery = useQuery(deploymentDAUs());
  const entitlementsQuery = useQuery(entitlements());

  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>
      <GeneralSettingsPageView
        deploymentOptions={deploymentValues.options}
        deploymentDAUs={deploymentDAUsQuery.data}
        deploymentDAUsError={deploymentDAUsQuery.error}
        entitlements={entitlementsQuery.data}
      />
    </>
  );
};

export default GeneralSettingsPage;
