import { useActor } from "@xstate/react"
import { useContext, useEffect } from "react"
import { XServiceContext } from "xServices/StateContext"
import { ServiceBannerView } from "./ServiceBannerView"

export const ServiceBanner: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [serviceBannerState, serviceBannerSend] = useActor(
    xServices.serviceBannerXService,
  )

  const { message, background_color, enabled } =
    serviceBannerState.context.serviceBanner

  /** Gets license data on app mount because LicenseBanner is mounted in App */
  useEffect(() => {
    serviceBannerSend("GET_BANNER")
  }, [serviceBannerSend])

  if (!enabled) {
    return null
  }

  if (enabled && message !== undefined && background_color !== undefined) {
    return (
      <ServiceBannerView message={message} backgroundColor={background_color} />
    )
  } else {
    return null
  }
}
