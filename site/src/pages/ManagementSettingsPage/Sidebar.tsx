import type { FC } from "react";
import { useQuery } from "react-query";
import { useLocation, useParams } from "react-router-dom";
import { organizationsPermissions } from "api/queries/organizations";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import {
  canEditOrganization,
  useOrganizationSettings,
} from "./ManagementSettingsLayout";
import { SidebarView } from "./SidebarView";

/**
 * A combined deployment settings and organization menu.
 *
 * This should only be used with multi-org support.  If multi-org support is
 * disabled or not licensed, this is the wrong sidebar to use.  See
 * DeploySettingsPage/Sidebar instead.
 */
export const Sidebar: FC = () => {
  const location = useLocation();
  const { permissions } = useAuthenticated();
  const { organizations } = useOrganizationSettings();
  const { organization: organizationName } = useParams() as {
    organization?: string;
  };

  const orgPermissionsQuery = useQuery(
    organizationsPermissions(organizations?.map((o) => o.id)),
  );

  // Sometimes a user can read an organization but cannot actually do anything
  // with it.  For now, these are filtered out so you only see organizations you
  // can manage in some way.
  const editableOrgs = organizations
    ?.map((org) => {
      const permissions = orgPermissionsQuery.data?.[org.id];
      return [org, permissions] as [Organization, AuthorizationResponse];
    })
    .filter(([_, permissions]) => {
      return canEditOrganization(permissions);
    });

  return (
    <SidebarView
      // Both activeSettings and activeOrganizationName could be be falsey if
      // the user is on /organizations but has no editable organizations to
      // which we can redirect.
      activeSettings={location.pathname.startsWith("/deployment")}
      activeOrganizationName={organizationName}
      organizations={editableOrgs}
      permissions={permissions}
    />
  );
};
