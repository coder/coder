import { useActor } from "@xstate/react"
import React from "react"

import { Navigate } from "react-router-dom"
import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { userXService } from "../xServices/user/userXService"

export const IndexPage: React.FC = () => {
  const [userState] = useActor(userXService)

  if (userState.matches('signedIn')) {
    return <Navigate to="/projects" replace />
  } else if (userState.matches('signedOut')) {
    return <Navigate to="/login" />
  } else {
    return <FullScreenLoader />
  }

}
