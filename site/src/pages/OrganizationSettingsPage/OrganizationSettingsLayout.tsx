import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet, useLocation, useParams } from "react-router-dom";
import { myOrganizations } from "api/queries/users";
import type { Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { useDashboard } from "modules/dashboard/useDashboard";
import NotFoundPage from "pages/404Page/404Page";
import { Sidebar } from "./Sidebar";

type OrganizationSettingsContextValue = {
  currentOrganizationId?: string;
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

export const OrganizationSettingsLayout: FC = () => {
  const location = useLocation();
  const { permissions } = useAuthenticated();
  const { experiments } = useDashboard();
  const { organization } = useParams() as { organization: string };
  const organizationsQuery = useQuery(myOrganizations());

  const multiOrgExperimentEnabled = experiments.includes("multi-organization");

  const inOrganizationSettings =
    location.pathname.startsWith("/organizations") &&
    location.pathname !== "/organizations/new";

  if (!multiOrgExperimentEnabled) {
    return <NotFoundPage />;
  }

  return (
    <RequirePermission isFeatureVisible={permissions.viewDeploymentValues}>
      <Margins>
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
          {organizationsQuery.data ? (
            <OrganizationSettingsContext.Provider
              value={{
                currentOrganizationId: !inOrganizationSettings
                  ? undefined
                  : !organization
                    ? organizationsQuery.data[0]?.id
                    : organizationsQuery.data.find(
                        (org) => org.name === organization,
                      )?.id,
                organizations: organizationsQuery.data,
              }}
            >
              <Sidebar />
              <main css={{ width: "100%" }}>
                <Suspense fallback={<Loader />}>
                  <Outlet />
                </Suspense>
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
