import { useSelector } from "@xstate/react"
import { Entitlements } from "api/typesGenerated"
import { useContext } from "react"
import { XServiceContext } from "xServices/StateContext"

export const useEntitlements = (): Entitlements => {
  const xServices = useContext(XServiceContext)
  const entitlements = useSelector(
    xServices.entitlementsXService,
    (state) => state.context.entitlements,
  )

  return entitlements
}
