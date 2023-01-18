import { shallowEqual, useActor, useSelector } from "@xstate/react"
import { useContext, FC } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceState] = useActor(xServices.appearanceXService)
  const [authState, authSend] = useActor(xServices.authXService)
  const [buildInfoState] = useActor(xServices.buildInfoXService)
  const { me, permissions } = authState.context
  const featureVisibility = useSelector(
    xServices.entitlementsXService,
    selectFeatureVisibility,
    shallowEqual,
  )
  const canViewAuditLog =
    featureVisibility["audit_log"] && Boolean(permissions?.viewAuditLog)
  const canViewDeployment = Boolean(permissions?.viewDeploymentConfig)
  const onSignOut = () => authSend("SIGN_OUT")

  return (
    <NavbarView
      user={me}
      logo_url={appearanceState.context.appearance.logo_url}
      buildInfo={buildInfoState.context.buildInfo}
      onSignOut={onSignOut}
      canViewAuditLog={canViewAuditLog}
      canViewDeployment={canViewDeployment}
    />
  )
}
