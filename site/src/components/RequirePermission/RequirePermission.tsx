import { FC, PropsWithChildren } from "react"
import { Navigate } from "react-router"

export type RequirePermissionProps = PropsWithChildren<{
  children: JSX.Element
  isFeatureVisible: boolean
}>

/**
 * Wraps routes that are available based on RBAC or licensing.
 */
export const RequirePermission: FC<RequirePermissionProps> = ({ children, isFeatureVisible }) => {
  if (!isFeatureVisible) {
    return <Navigate to="/workspaces" />
  } else {
    return children
  }
}
