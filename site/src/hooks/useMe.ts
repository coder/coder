import { User } from "api/typesGenerated"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { selectUser } from "xServices/auth/authSelectors"

export const useMe = (): User => {
  const [authState] = useAuth()
  const me = selectUser(authState)

  if (!me) {
    throw new Error("User not found.")
  }

  return me
}
