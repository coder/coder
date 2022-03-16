import { useActor } from "@xstate/react"
import React from "react"
import { Navigate, useLocation } from "react-router"
import { userXService } from "../../../xServices/user/userXService"
import { FullScreenLoader } from "../Loader/FullScreenLoader"

export interface RequireAuthProps {
  children: JSX.Element
}

export const RequireAuth: React.FC<RequireAuthProps> = ({ children }) => {
  const [userState] = useActor(userXService)
  const location = useLocation()

  if (userState.matches("signedOut") || !userState.context.me) {
    return <Navigate to={"/login?redirect=" + encodeURIComponent(location.pathname)} />
  } else if (userState.hasTag("loading")) {
    return <FullScreenLoader />
  } else {
    return children
  }
}

