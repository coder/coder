import { useActor } from "@xstate/react"
import { useContext } from "react"
import { AuthContext } from "xServices/auth/authXService"
import { XServiceContext } from "xServices/StateContext"

export const usePermissions = (): NonNullable<AuthContext["permissions"]> => {
  const xServices = useContext(XServiceContext)
  const [authState, _] = useActor(xServices.authXService)
  const { permissions } = authState.context
  if (!permissions) {
    throw new Error("Permissions are not loaded yet.")
  }
  return permissions
}
