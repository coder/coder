import { useActor, useSelector } from "@xstate/react"
import React, { useContext } from "react"
import { selectLicenseVisibility } from "../../xServices/license/licenseSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me } = authState.context
  const onSignOut = () => authSend("SIGN_OUT")

  const showAuditLog = useSelector(xServices.licenseXService, selectLicenseVisibility)["audit"]

  return <NavbarView user={me} onSignOut={onSignOut} showAuditLog={showAuditLog} />
}
