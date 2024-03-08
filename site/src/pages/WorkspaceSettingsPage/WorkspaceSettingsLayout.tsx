import { createContext, type FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router-dom";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import type { Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

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
        <Stack css={{ padding: "48px 0" }} direction="row" spacing={10}>
          {isError ? (
            <ErrorAlert error={error} />
          ) : (
            <WorkspaceSettings.Provider value={workspace}>
              <Sidebar workspace={workspace} username={username} />
              <Suspense fallback={<Loader />}>
                <main css={{ width: "100%" }}>
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
