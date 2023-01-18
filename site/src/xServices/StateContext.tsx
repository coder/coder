import { useInterpret } from "@xstate/react"
import { createContext, FC, ReactNode } from "react"
import { ActorRefFrom } from "xstate"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { entitlementsMachine } from "./entitlements/entitlementsXService"
import { appearanceMachine } from "./appearance/appearanceXService"

interface XServiceContextType {
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  entitlementsXService: ActorRefFrom<typeof entitlementsMachine>
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
        buildInfoXService: useInterpret(buildInfoMachine),
        entitlementsXService: useInterpret(entitlementsMachine),
        appearanceXService: useInterpret(appearanceMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
