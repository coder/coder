import { type FC, type ReactNode } from "react";
import { Navigate } from "react-router-dom";

export interface RequirePermissionProps {
  children?: ReactNode;
  isFeatureVisible: boolean;
}

/**
 * Wraps routes that are available based on RBAC or licensing.
 */
export const RequirePermission: FC<RequirePermissionProps> = ({
  children,
  isFeatureVisible,
}) => {
  if (!isFeatureVisible) {
    return <Navigate to="/workspaces" />;
  } else {
    return <>{children}</>;
  }
};
