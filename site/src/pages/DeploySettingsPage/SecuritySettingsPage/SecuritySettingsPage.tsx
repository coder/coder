import { useDashboard } from "components/Dashboard/DashboardProvider"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { SecuritySettingsPageView } from "./SecuritySettingsPageView"

const SecuritySettingsPage: FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()
  const { entitlements } = useDashboard()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Security Settings")}</title>
      </Helmet>

      <SecuritySettingsPageView
        deploymentConfig={deploymentConfig}
        featureAuditLogEnabled={entitlements.features["audit_log"].enabled}
        featureBrowserOnlyEnabled={
          entitlements.features["browser_only"].enabled
        }
      />
    </>
  )
}

export default SecuritySettingsPage
