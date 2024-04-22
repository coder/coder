import type { FC } from "react";
import { useQuery } from "react-query";
import { buildInfo } from "api/queries/buildInfo";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useProxy } from "contexts/ProxyContext";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "../useFeatureVisibility";
import { NavbarView } from "./NavbarView";

export const Navbar: FC = () => {
  const { appearance } = useDashboard();
  const buildInfoQuery = useQuery(buildInfo());
  const { user: me, permissions, signOut } = useAuthenticated();
  const featureVisibility = useFeatureVisibility();
  const canViewAuditLog =
    featureVisibility["audit_log"] && Boolean(permissions.viewAuditLog);
  const canViewDeployment = Boolean(permissions.viewDeploymentValues);
  const canViewAllUsers = Boolean(permissions.readAllUsers);
  const proxyContextValue = useProxy();
  const canViewHealth = canViewDeployment;
  return (
    <NavbarView
      user={me}
      logo_url={appearance.config.logo_url}
      buildInfo={buildInfoQuery.data}
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
