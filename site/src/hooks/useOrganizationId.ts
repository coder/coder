import { useSelector } from "@xstate/react"
import { useContext } from "react"
import { selectOrgId } from "../xServices/auth/authSelectors"
import { XServiceContext } from "../xServices/StateContext"

export const useOrganizationId = (): string => {
  const xServices = useContext(XServiceContext)
  const organizationId = useSelector(xServices.authXService, selectOrgId)

  if (!organizationId) {
    throw new Error("No organization ID found")
  }

  return organizationId
}
