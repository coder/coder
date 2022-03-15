import { useActor } from "@xstate/react"
import React from "react"

import { Navigate } from "react-router-dom"
import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { userXService } from "../xServices/user/userXService"

export const IndexPage: React.FC = () => {
  const [userState] = useActor(userXService)
  const { me } = userState.context

  if (me) {
    // Once the user is logged in, just redirect them to /projects as the landing page
    return <Navigate to="/projects" replace />
  }

  return <FullScreenLoader />
}
