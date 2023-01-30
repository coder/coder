import { useAuth } from "components/AuthProvider/AuthProvider"
import { selectOrgId } from "../xServices/auth/authSelectors"

export const useOrganizationId = (): string => {
  const [authState] = useAuth()
  const organizationId = selectOrgId(authState)

  if (!organizationId) {
    throw new Error("No organization ID found")
  }

  return organizationId
}
