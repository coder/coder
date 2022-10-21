import { useInterpret } from "@xstate/react"
import { createContext, FC, ReactNode } from "react"
import { ActorRefFrom } from "xstate"
import { authMachine } from "./auth/authXService"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { deploymentFlagsMachine } from "./deploymentFlags/deploymentFlagsMachine"
import { entitlementsMachine } from "./entitlements/entitlementsXService"
import { siteRolesMachine } from "./roles/siteRolesXService"

interface XServiceContextType {
  authXService: ActorRefFrom<typeof authMachine>
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  entitlementsXService: ActorRefFrom<typeof entitlementsMachine>
  siteRolesXService: ActorRefFrom<typeof siteRolesMachine>
  // Since the info here is used by multiple deployment settings page and we don't want to refetch them every time
  deploymentFlagsXService: ActorRefFrom<typeof deploymentFlagsMachine>
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
  return (
    <XServiceContext.Provider
      value={{
        authXService: useInterpret(authMachine),
        buildInfoXService: useInterpret(buildInfoMachine),
        entitlementsXService: useInterpret(entitlementsMachine),
        siteRolesXService: useInterpret(siteRolesMachine),
        deploymentFlagsXService: useInterpret(deploymentFlagsMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
