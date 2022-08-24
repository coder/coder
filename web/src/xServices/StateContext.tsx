import { useInterpret } from "@xstate/react"
import { createContext, FC, ReactNode } from "react"
import { useNavigate } from "react-router"
import { ActorRefFrom } from "xstate"
import { authMachine } from "./auth/authXService"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { entitlementsMachine } from "./entitlements/entitlementsXService"
import { siteRolesMachine } from "./roles/siteRolesXService"
import { usersMachine } from "./users/usersXService"

interface XServiceContextType {
  authXService: ActorRefFrom<typeof authMachine>
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  entitlementsXService: ActorRefFrom<typeof entitlementsMachine>
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

export const XServiceProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const navigate = useNavigate()
  const redirectToUsersPage = () => {
    navigate("users")
  }
  const redirectToSetupPage = () => {
    navigate("setup")
  }

  return (
    <XServiceContext.Provider
      value={{
        authXService: useInterpret(() =>
          authMachine.withConfig({ actions: { redirectToSetupPage } }),
        ),
        buildInfoXService: useInterpret(buildInfoMachine),
        entitlementsXService: useInterpret(entitlementsMachine),
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
