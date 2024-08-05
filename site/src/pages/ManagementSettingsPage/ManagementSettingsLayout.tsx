import { type FC, Suspense } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";
import { deploymentConfig } from "api/queries/deployment";
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

type OrganizationSettingsValue = { organizations: Organization[] };

export const useOrganizationSettings = (): OrganizationSettingsValue => {
  const { organizations } = useDashboard();
  return { organizations };
};

export const ManagementSettingsLayout: FC = () => {
  const { permissions } = useAuthenticated();
  const { experiments } = useDashboard();
  const deploymentConfigQuery = useQuery(deploymentConfig());

  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  if (!multiOrgExperimentEnabled) {
    return <NotFoundPage />;
  }

  return (
    <RequirePermission isFeatureVisible={permissions.viewDeploymentValues}>
      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          <Sidebar />
          <main css={{ width: "100%" }}>
            {deploymentConfigQuery.data ? (
              <DeploySettingsContext.Provider
                value={{ deploymentValues: deploymentConfigQuery.data }}
              >
                <Suspense fallback={<Loader />}>
                  <Outlet />
                </Suspense>
              </DeploySettingsContext.Provider>
            ) : (
              <Loader />
            )}
          </main>
        </Stack>
      </Margins>
    </RequirePermission>
  );
};
