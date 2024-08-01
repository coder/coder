import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { organizationPermissions } from "api/queries/organizations";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { SidebarView } from "./SidebarView";

/**
 * A combined deployment settings and organization menu.
 *
 * This should only be used with multi-org support.  If multi-org support is
 * disabled or not licensed, this is the wrong sidebar to use.  See
 * DeploySettingsPage/Sidebar instead.
 */
export const Sidebar: FC = () => {
  const { permissions } = useAuthenticated();
  const { organizations } = useOrganizationSettings();
  const { organization: organizationName } = useParams() as {
    organization?: string;
  };

  // If there is no organization name, the settings page will load, and it will
  // redirect to the default organization, so eventually there will always be an
  // organization name.
  const activeOrganization = organizations?.find(
    (o) => o.name === organizationName,
  );
  const activeOrgPermissionsQuery = useQuery(
    organizationPermissions(activeOrganization?.id),
  );

  return (
    <SidebarView
      activeOrganization={activeOrganization}
      activeOrgPermissions={activeOrgPermissionsQuery.data}
      organizations={organizations}
      permissions={permissions}
    />
  );
};
