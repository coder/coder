import { useActor } from "@xstate/react"
import { useContext, useEffect } from "react"
import { XServiceContext } from "xServices/StateContext"
import { LicenseBannerView } from "./LicenseBannerView"

export const LicenseBanner: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [entitlementsState, entitlementsSend] = useActor(
    xServices.entitlementsXService,
  )
  const { errors, warnings } = entitlementsState.context.entitlements

  /** Gets license data on app mount because LicenseBanner is mounted in App */
  useEffect(() => {
    entitlementsSend("GET_ENTITLEMENTS")
  }, [entitlementsSend])

  if (errors.length > 0 || warnings.length > 0) {
    return <LicenseBannerView errors={errors} warnings={warnings} />
  } else {
    return null
  }
}
