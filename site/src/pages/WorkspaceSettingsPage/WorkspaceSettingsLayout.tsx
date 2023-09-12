import { makeStyles } from "@mui/styles";
import { Sidebar } from "./Sidebar";
import { Stack } from "components/Stack/Stack";
import { createContext, FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "../../utils/page";
import { Loader } from "components/Loader/Loader";
import { Outlet, useParams } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { workspaceByOwnerAndName } from "api/queries/workspace";
import { useQuery } from "@tanstack/react-query";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { type Workspace } from "api/typesGenerated";

const WorkspaceSettings = createContext<Workspace | undefined>(undefined);

export function useWorkspaceSettings() {
  const value = useContext(WorkspaceSettings);
  if (!value) {
    throw new Error(
      "This hook can only be used from a workspace settings page",
    );
  }

  return value;
}

export const WorkspaceSettingsLayout: FC = () => {
  const styles = useStyles();
  const params = useParams() as {
    workspace: string;
    username: string;
  };
  const workspaceName = params.workspace;
  const username = params.username.replace("@", "");
  const {
    data: workspace,
    error,
    isLoading,
    isError,
  } = useQuery(workspaceByOwnerAndName(username, workspaceName));

  if (isLoading) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspaceName, "Settings"])}</title>
      </Helmet>

      <Margins>
        <Stack className={styles.wrapper} direction="row" spacing={10}>
          {isError ? (
            <ErrorAlert error={error} />
          ) : (
            <WorkspaceSettings.Provider value={workspace}>
              <Sidebar workspace={workspace} username={username} />
              <Suspense fallback={<Loader />}>
                <main className={styles.content}>
                  <Outlet />
                </main>
              </Suspense>
            </WorkspaceSettings.Provider>
          )}
        </Stack>
      </Margins>
    </>
  );
};

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    width: "100%",
  },
}));
