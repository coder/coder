import React from "react"

import { Redirect } from "../components"
import { FullScreenLoader } from "../components/Loader/FullScreenLoader"
import { useUser } from "../contexts/UserContext"

const IndexPage: React.FC = () => {
  const { me } = useUser(/* redirectOnError */ true)

  if (me) {
    // Once the user is logged in, just redirect them to /projects as the landing page
    return <Redirect to="/projects" />
  }

  return <FullScreenLoader />
}

export default IndexPage
