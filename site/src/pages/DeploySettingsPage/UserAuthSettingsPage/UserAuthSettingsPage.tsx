import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import { useDeploySettings } from "../DeploySettingsLayout";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const UserAuthSettingsPage: FC = () => {
  const { deploymentValues } = useDeploySettings();

  return (
    <>
      <Helmet>
        <title>{pageTitle("User Authentication Settings")}</title>
      </Helmet>

      {deploymentValues ? (
        <UserAuthSettingsPageView options={deploymentValues.options} />
      ) : (
        <Loader />
      )}
    </>
  );
};

export default UserAuthSettingsPage;
