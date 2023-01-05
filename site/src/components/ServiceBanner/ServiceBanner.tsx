import { useActor } from "@xstate/react"
import { useContext } from "react"
import { XServiceContext } from "xServices/StateContext"
import { ServiceBannerView } from "./ServiceBannerView"

export const ServiceBanner: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceState] = useActor(xServices.appearanceXService)

  const { message, background_color, enabled } =
    appearanceState.context.appearance.service_banner

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
