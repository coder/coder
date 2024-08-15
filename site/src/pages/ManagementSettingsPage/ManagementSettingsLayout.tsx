import { deploymentConfig } from "api/queries/deployment";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, Suspense } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";
import { DeploySettingsContext } from "../DeploySettingsPage/DeploySettingsLayout";
import { Sidebar } from "./Sidebar";

type OrganizationSettingsValue = {
  organizations: Organization[];
};

export const useOrganizationSettings = (): OrganizationSettingsValue => {
  const { organizations } = useDashboard();
  return { organizations };
};

/**
 * Return true if the user can edit the organization settings or its members.
 */
export const canEditOrganization = (
  permissions: AuthorizationResponse | undefined,
) => {
  return (
    permissions !== undefined &&
    (permissions.editOrganization ||
      permissions.editMembers ||
      permissions.editGroups)
  );
};

/**
 * A multi-org capable settings page layout.
 *
 * If multi-org is not enabled or licensed, this is the wrong layout to use.
 * See DeploySettingsLayoutInner instead.
 */
export const ManagementSettingsLayout: FC = () => {
  const { permissions } = useAuthenticated();
  const deploymentConfigQuery = useQuery(
    // TODO: This is probably normally fine because we will not show links to
    //       pages that need this data, but if you manually visit the page you
    //       will see an endless loader when maybe we should show a "permission
    //       denied" error or at least a 404 instead.
    permissions.viewDeploymentValues ? deploymentConfig() : { enabled: false },
  );

  // The deployment settings page also contains users, audit logs, groups and
  // organizations, so this page must be visible if you can see any of these.
  const canViewDeploymentSettingsPage =
    permissions.viewDeploymentValues ||
    permissions.viewAllUsers ||
    permissions.editAnyOrganization ||
    permissions.viewAnyAuditLog;

  return (
    <RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          <Sidebar />
          <main css={{ width: "100%" }}>
            <DeploySettingsContext.Provider
              value={{
                deploymentValues: deploymentConfigQuery.data,
              }}
            >
              <Suspense fallback={<Loader />}>
                <Outlet />
              </Suspense>
            </DeploySettingsContext.Provider>
          </main>
        </Stack>
      </Margins>
    </RequirePermission>
  );
};
