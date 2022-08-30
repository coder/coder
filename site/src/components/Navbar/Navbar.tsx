import { shallowEqual, useActor, useSelector } from "@xstate/react"
import { FeatureNames } from "api/types"
import React, { useContext } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState, authSend] = useActor(xServices.authXService)
  const { me, permissions } = authState.context
  const featureVisibility = useSelector(
    xServices.entitlementsXService,
    selectFeatureVisibility,
    shallowEqual,
  )
  const canViewAuditLog = featureVisibility[FeatureNames.AuditLog] && !!permissions?.viewAuditLog
  const onSignOut = () => authSend("SIGN_OUT")

  return <NavbarView user={me} onSignOut={onSignOut} canViewAuditLog={canViewAuditLog} />
}
