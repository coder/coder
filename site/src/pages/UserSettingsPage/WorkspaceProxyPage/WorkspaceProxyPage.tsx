import { FC, PropsWithChildren } from "react"
import { Section } from "components/SettingsLayout/Section"
import { WorkspaceProxyView } from "./WorkspaceProxyView"
import makeStyles from "@mui/styles/makeStyles"
import { displayError } from "components/GlobalSnackbar/utils"
import { useProxy } from "contexts/ProxyContext"

export const WorkspaceProxyPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()

  const description =
    "Workspace proxies are used to reduce the latency of connections to your workspaces." +
    "To get the best experience, choose the workspace proxy that is closest to you." +
    "This selection only affects browser connections to your workspace."

  const {
    proxyLatencies,
    proxies,
    error: proxiesError,
    isFetched: proxiesFetched,
    isLoading: proxiesLoading,
    proxy,
    setProxy,
  } = useProxy()

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
        onSelect={(proxy) => {
          if (!proxy.healthy) {
            displayError("Please select a healthy workspace proxy.")
            return
          }

          setProxy(proxy)
        }}
      />
    </Section>
  )
}

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
}))

export default WorkspaceProxyPage
