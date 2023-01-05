import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import { AppearanceConfig } from "api/typesGenerated"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { XServiceContext } from "xServices/StateContext"
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView"

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const AppearanceSettingsPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceXService, appearanceSend] = useActor(
    xServices.appearanceXService,
  )
  const [entitlementsState] = useActor(xServices.entitlementsXService)
  const appearance = appearanceXService.context.appearance

  const isEntitled =
    entitlementsState.context.entitlements.features[FeatureNames.Appearance]
      .entitlement !== "not_entitled"

  const updateAppearance = (
    newConfig: Partial<AppearanceConfig>,
    preview: boolean,
  ) => {
    const newAppearance = {
      ...appearance,
      ...newConfig,
    }
    if (preview) {
      appearanceSend({
        type: "SET_PREVIEW_APPEARANCE",
        appearance: newAppearance,
      })
      return
    }
    appearanceSend({
      type: "SET_APPEARANCE",
      appearance: newAppearance,
    })
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Appearance Settings")}</title>
      </Helmet>

      <AppearanceSettingsPageView
        appearance={appearance}
        isEntitled={isEntitled}
        updateAppearance={updateAppearance}
      />
    </>
  )
}

export default AppearanceSettingsPage
