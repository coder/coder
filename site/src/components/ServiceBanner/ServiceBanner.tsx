import { useActor } from "@xstate/react"
import { useContext, useEffect } from "react"
import { XServiceContext } from "xServices/StateContext"
import { ServiceBannerView } from "./ServiceBannerView"

export const ServiceBanner: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceState, appearanceSend] = useActor(
    xServices.appearanceXService,
  )
  const [authState] = useActor(xServices.authXService)
  const { message, background_color, enabled } =
    appearanceState.context.appearance.service_banner

  useEffect(() => {
    if (authState.matches("signedIn")) {
      appearanceSend("GET_APPEARANCE")
    }
  }, [appearanceSend, authState])

  if (!enabled) {
    return null
  }

  if (message !== undefined && background_color !== undefined) {
    return (
      <ServiceBannerView
        message={message}
        backgroundColor={background_color}
        preview={appearanceState.context.preview}
      />
    )
  } else {
    return null
  }
}
