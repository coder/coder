import { shallowEqual, useActor, useSelector } from "@xstate/react"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { useMe } from "hooks/useMe"
import { usePermissions } from "hooks/usePermissions"
import { useContext, FC } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { NavbarView } from "../NavbarView/NavbarView"

export const Navbar: FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceState] = useActor(xServices.appearanceXService)
  const [buildInfoState] = useActor(xServices.buildInfoXService)
  const [_, authSend] = useAuth()
  const me = useMe()
  const permissions = usePermissions()
  const featureVisibility = useSelector(
    xServices.entitlementsXService,
    selectFeatureVisibility,
    shallowEqual,
  )
  const canViewAuditLog =
    featureVisibility["audit_log"] && Boolean(permissions.viewAuditLog)
  const canViewDeployment = Boolean(permissions.viewDeploymentConfig)
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
