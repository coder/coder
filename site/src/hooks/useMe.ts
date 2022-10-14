import { useSelector } from "@xstate/react"
import { User } from "api/typesGenerated"
import { useContext } from "react"
import { selectUser } from "xServices/auth/authSelectors"
import { XServiceContext } from "xServices/StateContext"

export const useMe = (): User => {
  const xServices = useContext(XServiceContext)
  const me = useSelector(xServices.authXService, selectUser)

  if (!me) {
    throw new Error("User not found.")
  }

  return me
}
