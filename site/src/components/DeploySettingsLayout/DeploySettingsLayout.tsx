import { makeStyles } from "@mui/styles";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { Sidebar } from "./Sidebar";
import { createContext, Suspense, useContext, FC } from "react";
import { useMachine } from "@xstate/react";
import { Loader } from "components/Loader/Loader";
import { DAUsResponse } from "api/typesGenerated";
import { deploymentConfigMachine } from "xServices/deploymentConfig/deploymentConfigMachine";
import { RequirePermission } from "components/RequirePermission/RequirePermission";
import { usePermissions } from "hooks/usePermissions";
import { Outlet } from "react-router-dom";
import { DeploymentConfig } from "api/api";

type DeploySettingsContextValue = {
  deploymentValues: DeploymentConfig;
  getDeploymentValuesError: unknown;
  deploymentDAUs?: DAUsResponse;
  getDeploymentDAUsError: unknown;
};

const DeploySettingsContext = createContext<
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
  const [state] = useMachine(deploymentConfigMachine);
  const styles = useStyles();
  const {
    deploymentValues,
    deploymentDAUs,
    getDeploymentValuesError,
    getDeploymentDAUsError,
  } = state.context;
  const permissions = usePermissions();

  return (
    <RequirePermission isFeatureVisible={permissions.viewDeploymentValues}>
      <Margins>
        <Stack className={styles.wrapper} direction="row" spacing={6}>
          <Sidebar />
          <main className={styles.content}>
            {deploymentValues ? (
              <DeploySettingsContext.Provider
                value={{
                  deploymentValues,
                  getDeploymentValuesError,
                  deploymentDAUs,
                  getDeploymentDAUsError,
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
        </Stack>
      </Margins>
    </RequirePermission>
  );
};

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    maxWidth: 800,
    width: "100%",
  },
}));
