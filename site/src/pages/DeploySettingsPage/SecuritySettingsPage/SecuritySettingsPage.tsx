import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { useContext, FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { XServiceContext } from "xServices/StateContext"
import { SecuritySettingsPageView } from "./SecuritySettingsPageView"

const SecuritySettingsPage: FC = () => {
  const { deploymentConfig: deploymentConfig } = useDeploySettings()
  const xServices = useContext(XServiceContext)
  const [entitlementsState] = useActor(xServices.entitlementsXService)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Security Settings")}</title>
      </Helmet>

      <SecuritySettingsPageView
        deploymentConfig={deploymentConfig}
        featureAuditLogEnabled={
          entitlementsState.context.entitlements.features[FeatureNames.AuditLog]
            .enabled
        }
        featureBrowserOnlyEnabled={
          entitlementsState.context.entitlements.features[
            FeatureNames.BrowserOnly
          ].enabled
        }
      />
    </>
  )
}

export default SecuritySettingsPage
