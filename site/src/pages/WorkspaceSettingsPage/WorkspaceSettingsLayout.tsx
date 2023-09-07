import { makeStyles } from "@mui/styles";
import { Sidebar } from "./Sidebar";
import { Stack } from "components/Stack/Stack";
import { createContext, FC, Suspense, useContext } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "../../utils/page";
import { Loader } from "components/Loader/Loader";
import { Outlet, useParams } from "react-router-dom";
import { Margins } from "components/Margins/Margins";
import { getWorkspaceByOwnerAndName } from "api/api";
import { useQuery } from "@tanstack/react-query";

const fetchWorkspaceSettings = async (owner: string, name: string) => {
  const workspace = await getWorkspaceByOwnerAndName(owner, name);

  return {
    workspace,
  };
};

const useWorkspace = (owner: string, name: string) => {
  return useQuery({
    queryKey: ["workspace", name, "settings"],
    queryFn: () => fetchWorkspaceSettings(owner, name),
  });
};

const WorkspaceSettingsContext = createContext<
  Awaited<ReturnType<typeof fetchWorkspaceSettings>> | undefined
>(undefined);

export const useWorkspaceSettingsContext = () => {
  const context = useContext(WorkspaceSettingsContext);

  if (!context) {
    throw new Error(
      "useWorkspaceSettingsContext must be used within a WorkspaceSettingsContext.Provider",
    );
  }

  return context;
};

export const WorkspaceSettingsLayout: FC = () => {
  const styles = useStyles();
  const params = useParams() as {
    workspace: string;
    username: string;
  };
  const workspaceName = params.workspace;
  const username = params.username.replace("@", "");
  const { data: settings } = useWorkspace(username, workspaceName);

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspaceName, "Settings"])}</title>
      </Helmet>

      {settings ? (
        <WorkspaceSettingsContext.Provider value={settings}>
          <Margins>
            <Stack className={styles.wrapper} direction="row" spacing={10}>
              <Sidebar workspace={settings.workspace} username={username} />
              <Suspense fallback={<Loader />}>
                <main className={styles.content}>
                  <Outlet />
                </main>
              </Suspense>
            </Stack>
          </Margins>
        </WorkspaceSettingsContext.Provider>
      ) : (
        <Loader />
      )}
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
