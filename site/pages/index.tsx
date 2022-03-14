import React from "react"

import { Navigate } from "react-router-dom"
import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { useUser } from "../contexts/UserContext"

export const IndexPage: React.FC = () => {
  const { me } = useUser(/* redirectOnError */ true)

  if (me) {
    // Once the user is logged in, just redirect them to /projects as the landing page
    return <Navigate to="/projects" replace />
  }

  return <FullScreenLoader />
}
