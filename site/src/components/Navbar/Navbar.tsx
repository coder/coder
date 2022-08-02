import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: React.FC<React.PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, permissions } = authState.context
  const onSignOut = () => authSend("SIGN_OUT")

  return (
    <NavbarView
      user={me}
      onSignOut={onSignOut}
      canViewAuditLog={permissions?.viewAuditLog ?? false}
    />
  )
}
