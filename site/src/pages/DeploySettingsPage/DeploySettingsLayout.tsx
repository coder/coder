import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet } from "react-router-dom";
import type { DeploymentConfig } from "api/api";
import { deploymentConfig } from "api/queries/deployment";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { ManagementSettingsLayout } from "pages/ManagementSettingsPage/ManagementSettingsLayout";
import { Sidebar } from "./Sidebar";

type DeploySettingsContextValue = {
  deploymentValues: DeploymentConfig | undefined;
};

export const DeploySettingsContext = createContext<
  DeploySettingsContextValue | undefined
>(undefined);

export const useDeploySettings = (): DeploySettingsContextValue => {
  const context = useContext(DeploySettingsContext);
  if (!context) {
    throw new Error(
      "useDeploySettings should be used inside of DeploySettingsLayout",
    );
  }
  return context;
};

export const DeploySettingsLayout: FC = () => {
  const { experiments } = useDashboard();

  const feats = useFeatureVisibility();
  const canViewOrganizations =
    feats.multiple_organizations && experiments.includes("multi-organization");

  return canViewOrganizations ? (
    <ManagementSettingsLayout />
  ) : (
    <DeploySettingsLayoutInner />
  );
};

const DeploySettingsLayoutInner: FC = () => {
  const deploymentConfigQuery = useQuery(deploymentConfig());
  const { permissions } = useAuthenticated();

  return (
    <RequirePermission isFeatureVisible={permissions.viewDeploymentValues}>
      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          <Sidebar />
          <main css={{ maxWidth: 800, width: "100%" }}>
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
