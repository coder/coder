import { useActor } from "@xstate/react"
import { useContext } from "react"
import { XServiceContext } from "../xServices/StateContext"

export const useOrganizationId = (): string => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const organizationId = authState.context.me?.organization_ids[0]

  if (!organizationId) {
    throw new Error("No organization ID found")
  }

  return organizationId
}
