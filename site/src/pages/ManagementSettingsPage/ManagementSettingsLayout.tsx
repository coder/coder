import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";
import { deploymentConfig } from "api/queries/deployment";
import { organizations } from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import NotFoundPage from "pages/404Page/404Page";
import { DeploySettingsContext } from "../DeploySettingsPage/DeploySettingsLayout";
import { Sidebar } from "./Sidebar";

type OrganizationSettingsContextValue = {
  organizations: Organization[];
};

const OrganizationSettingsContext = createContext<
  OrganizationSettingsContextValue | undefined
>(undefined);

export const useOrganizationSettings = (): OrganizationSettingsContextValue => {
  const context = useContext(OrganizationSettingsContext);
  if (!context) {
    throw new Error(
      "useOrganizationSettings should be used inside of OrganizationSettingsLayout",
    );
  }
  return context;
};

export const ManagementSettingsLayout: FC = () => {
  const { permissions } = useAuthenticated();
  const { experiments } = useDashboard();
  const deploymentConfigQuery = useQuery({
    ...deploymentConfig(),
    enabled: permissions.viewDeploymentValues,
  });
  const organizationsQuery = useQuery(organizations());

  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  // ManagementSettingsLayout includes both site wide and organization specific settings.
  // An actor can visit this page if they can view at least some subset of these settings.
  const viewSettingsPage =
    permissions.viewDeploymentValues ||
    permissions.createAnyGroup ||
    permissions.viewAnyAuditLog;

  if (!multiOrgExperimentEnabled) {
    return <NotFoundPage />;
  }

  return (
    <RequirePermission isFeatureVisible={viewSettingsPage}>
      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          {organizationsQuery.data ? (
            <OrganizationSettingsContext.Provider
              value={{ organizations: organizationsQuery.data }}
            >
              <Sidebar />
              <main css={{ width: "100%" }}>
                {/* TODO: 
                This will spin forever if the user does not have 
                "permissions.viewDeploymentValues" permission. We need 
                */}
                {deploymentConfigQuery.data ? (
                  <DeploySettingsContext.Provider
                    value={{
                      deploymentValues: deploymentConfigQuery.data,
                    }}
                  >
                    <Suspense fallback={<Loader />}>
                      <Outlet />
                    </Suspense>
                  </DeploySettingsContext.Provider>
                ) : (
                  <Loader />
                )}
              </main>
            </OrganizationSettingsContext.Provider>
          ) : (
            <Loader />
          )}
        </Stack>
      </Margins>
    </RequirePermission>
  );
};
