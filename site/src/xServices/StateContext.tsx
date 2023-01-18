import { useInterpret } from "@xstate/react"
import { createContext, FC, ReactNode } from "react"
import { ActorRefFrom } from "xstate"
import { authMachine } from "./auth/authXService"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { entitlementsMachine } from "./entitlements/entitlementsXService"
import { experimentsMachine } from "./experiments/experimentsMachine"
import { appearanceMachine } from "./appearance/appearanceXService"

interface XServiceContextType {
  authXService: ActorRefFrom<typeof authMachine>
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  entitlementsXService: ActorRefFrom<typeof entitlementsMachine>
  experimentsXService: ActorRefFrom<typeof experimentsMachine>
  appearanceXService: ActorRefFrom<typeof appearanceMachine>
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
        experimentsXService: useInterpret(experimentsMachine),
        appearanceXService: useInterpret(appearanceMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
