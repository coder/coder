import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "./NavbarView"

export const Navbar: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [userState, userSend] = useActor(xServices.userXService)
  const { me } = userState.context
  const onSignOut = () => userSend("SIGN_OUT")

  return <NavbarView user={me} onSignOut={onSignOut} />
}
