import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { Navigate, useLocation } from "react-router"
import { XServiceContext } from "../../xServices/StateContext"
import { FullScreenLoader } from "../Loader/FullScreenLoader"

export interface RequireAuthProps {
  children: JSX.Element
}

export const RequireAuth: React.FC<RequireAuthProps> = ({ children }) => {
  const xServices = useContext(XServiceContext)
  const [userState] = useActor(xServices.userXService)
  const location = useLocation()

  if (userState.matches("signedOut") || !userState.context.me) {
    return <Navigate to={"/login?redirect=" + encodeURIComponent(location.pathname)} />
  } else if (userState.hasTag("loading")) {
    return <FullScreenLoader />
  } else {
    return children
  }
}
