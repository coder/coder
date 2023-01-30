import { useAuth } from "components/AuthProvider/AuthProvider"
import { AuthContext } from "xServices/auth/authXService"

export const usePermissions = (): NonNullable<AuthContext["permissions"]> => {
  const [authState] = useAuth()
  const { permissions } = authState.context

  if (!permissions) {
    throw new Error("Permissions are not loaded yet.")
  }

  return permissions
}
