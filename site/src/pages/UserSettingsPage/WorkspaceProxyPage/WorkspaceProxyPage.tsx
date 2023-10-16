import { FC, PropsWithChildren } from "react";
import { Section } from "components/SettingsLayout/Section";
import { WorkspaceProxyView } from "./WorkspaceProxyView";
import makeStyles from "@mui/styles/makeStyles";
import { useProxy } from "contexts/ProxyContext";

export const WorkspaceProxyPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles();

  const description =
    "Workspace proxies improve terminal and web app connections to workspaces.";

  const {
    proxyLatencies,
    proxies,
    error: proxiesError,
    isFetched: proxiesFetched,
    isLoading: proxiesLoading,
    proxy,
  } = useProxy();

  return (
    <Section
      title="Workspace Proxies"
      className={styles.section}
      description={description}
      layout="fluid"
    >
      <WorkspaceProxyView
        proxyLatencies={proxyLatencies}
        proxies={proxies}
        isLoading={proxiesLoading}
        hasLoaded={proxiesFetched}
        getWorkspaceProxiesError={proxiesError}
        preferredProxy={proxy.proxy}
      />
    </Section>
  );
};

const useStyles = makeStyles((theme) => ({
  section: {
    "& code": {
      background: theme.palette.divider,
      fontSize: 12,
      padding: "2px 4px",
      color: theme.palette.text.primary,
      borderRadius: 2,
    },
  },
}));

export default WorkspaceProxyPage;
