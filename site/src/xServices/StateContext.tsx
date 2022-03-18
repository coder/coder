import React, { createContext } from "react"
import { useInterpret } from "@xstate/react"
import { ActorRefFrom } from "xstate"
import { userMachine } from "./user/userXService"

interface XServiceContextType {
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
  const userXService = useInterpret(userMachine, { devTools: true })

  return <XServiceContext.Provider value={{ userXService }}>{children}</XServiceContext.Provider>
}
