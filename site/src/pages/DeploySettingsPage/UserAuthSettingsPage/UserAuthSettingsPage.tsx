import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView"

const UserAuthSettingsPage: FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("User Authentication Settings")}</title>
      </Helmet>

      <UserAuthSettingsPageView deploymentConfig={deploymentConfig} />
    </>
  )
}

export default UserAuthSettingsPage
