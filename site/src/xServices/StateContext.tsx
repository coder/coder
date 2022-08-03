import { useInterpret } from "@xstate/react"
import React, { createContext } from "react"
import { useNavigate } from "react-router"
import { ActorRefFrom } from "xstate"
import { authMachine } from "./auth/authXService"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { siteRolesMachine } from "./roles/siteRolesXService"
import { usersMachine } from "./users/usersXService"

interface XServiceContextType {
  authXService: ActorRefFrom<typeof authMachine>
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  usersXService: ActorRefFrom<typeof usersMachine>
  siteRolesXService: ActorRefFrom<typeof siteRolesMachine>
}

/**
 * Consuming this Context will not automatically cause rerenders because
 * the xServices in it are static references.
 *
 * To use one of the xServices, `useActor` will access all its state
 * (causing re-renders for any changes to that one xService) and
 * `useSelector` will access just one piece of state.
 */
export const XServiceContext = createContext({} as XServiceContextType)

export const XServiceProvider: React.FC<React.PropsWithChildren<unknown>> = ({ children }) => {
  const navigate = useNavigate()
  const redirectToUsersPage = () => {
    navigate("users")
  }

  return (
    <XServiceContext.Provider
      value={{
        authXService: useInterpret(authMachine),
        buildInfoXService: useInterpret(buildInfoMachine),
        usersXService: useInterpret(() =>
          usersMachine.withConfig({ actions: { redirectToUsersPage } }),
        ),
        siteRolesXService: useInterpret(siteRolesMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
