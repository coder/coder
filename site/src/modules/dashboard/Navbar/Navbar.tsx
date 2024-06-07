import type { FC } from "react";
import { useQuery } from "react-query";
import { buildInfo } from "api/queries/buildInfo";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useProxy } from "contexts/ProxyContext";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "../useFeatureVisibility";
import { NavbarView } from "./NavbarView";

export const Navbar: FC = () => {
  const { metadata } = useEmbeddedMetadata();
  const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));

  const { appearance, experiments } = useDashboard();
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
      logo_url={appearance.logo_url}
      buildInfo={buildInfoQuery.data}
      supportLinks={appearance.support_links}
      onSignOut={signOut}
      canViewDeployment={canViewDeployment}
      canViewOrganizations={experiments.includes("multi-organization")}
      canViewAllUsers={canViewAllUsers}
      canViewHealth={canViewHealth}
      canViewAuditLog={canViewAuditLog}
      proxyContextValue={proxyContextValue}
    />
  );
};
