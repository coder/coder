import { useInterpret } from "@xstate/react"
import React, { createContext } from "react"
import { ActorRefFrom } from "xstate"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { userMachine } from "./user/userXService"

interface XServiceContextType {
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  userXService: ActorRefFrom<typeof userMachine>
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

export const XServiceProvider: React.FC = ({ children }) => {
  return (
    <XServiceContext.Provider
      value={{
        buildInfoXService: useInterpret(buildInfoMachine),
        userXService: useInterpret(userMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
