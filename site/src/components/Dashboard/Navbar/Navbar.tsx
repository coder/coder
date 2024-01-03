import { type FC } from "react";
import { useAuth } from "contexts/AuthProvider/AuthProvider";
import { useProxy } from "contexts/ProxyContext";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { NavbarView } from "./NavbarView";

export const Navbar: FC = () => {
  const { appearance, buildInfo } = useDashboard();
  const { signOut } = useAuth();
  const me = useMe();
  const permissions = usePermissions();
  const featureVisibility = useFeatureVisibility();
  const canViewAuditLog =
    featureVisibility["audit_log"] && Boolean(permissions.viewAuditLog);
  const canViewDeployment = Boolean(permissions.viewDeploymentValues);
  const canViewAllUsers = Boolean(permissions.readAllUsers);
  const proxyContextValue = useProxy();
  const dashboard = useDashboard();
  const canViewHealth =
    canViewDeployment &&
    dashboard.experiments.includes("deployment_health_page");

  return (
    <NavbarView
      user={me}
      logo_url={appearance.config.logo_url}
      buildInfo={buildInfo}
      supportLinks={appearance.config.support_links}
      onSignOut={signOut}
      canViewAuditLog={canViewAuditLog}
      canViewDeployment={canViewDeployment}
      canViewAllUsers={canViewAllUsers}
      canViewHealth={canViewHealth}
      proxyContextValue={proxyContextValue}
    />
  );
};
