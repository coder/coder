import { useAuth } from "components/AuthProvider/AuthProvider";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { FC } from "react";
import { NavbarView } from "./NavbarView";
import { useProxy } from "contexts/ProxyContext";

export const Navbar: FC = () => {
  const { appearance, buildInfo } = useDashboard();
  const [_, authSend] = useAuth();
  const me = useMe();
  const permissions = usePermissions();
  const featureVisibility = useFeatureVisibility();
  const canViewAuditLog =
    featureVisibility["audit_log"] && Boolean(permissions.viewAuditLog);
  const canViewDeployment = Boolean(permissions.viewDeploymentValues);
  const canViewAllUsers = Boolean(permissions.readAllUsers);
  const onSignOut = () => authSend("SIGN_OUT");
  const proxyContextValue = useProxy();
  const dashboard = useDashboard();

  return (
    <NavbarView
      user={me}
      logo_url={appearance.config.logo_url}
      buildInfo={buildInfo}
      supportLinks={appearance.config.support_links}
      onSignOut={onSignOut}
      canViewAuditLog={canViewAuditLog}
      canViewDeployment={canViewDeployment}
      canViewAllUsers={canViewAllUsers}
      proxyContextValue={
        dashboard.experiments.includes("moons") ? proxyContextValue : undefined
      }
    />
  );
};
