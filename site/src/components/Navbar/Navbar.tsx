import { useActor, useSelector } from "@xstate/react"
import React, { useContext } from "react"
import { selectPermissions } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me } = authState.context
  const permissions = useSelector(xServices.authXService, selectPermissions)
  // When we have more options in the admin dropdown we may want to check this
  // for more permissions
  const displayAdminDropdown = !!permissions?.updateUsers
  const onSignOut = () => authSend("SIGN_OUT")

  return <NavbarView user={me} onSignOut={onSignOut} displayAdminDropdown={displayAdminDropdown} />
}
