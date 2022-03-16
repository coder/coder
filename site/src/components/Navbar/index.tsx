import React from "react"
import { useActor } from "@xstate/react"
import { userXService } from "../../xServices/user/userXService"
import { NavbarView } from "./NavbarView"

export const Navbar: React.FC = () => {
  const [userState, userSend] = useActor(userXService)
  const { me } = userState.context
  const onSignOut = () => userSend("SIGN_OUT")

  return <NavbarView user={me} onSignOut={onSignOut} />
}
