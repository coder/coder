import { FC, PropsWithChildren } from "react"
import { Section } from "components/SettingsLayout/Section"
import { WorkspaceProxyView } from "./WorkspaceProxyView"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useWorkspaceProxiesData } from "./hooks"
import { displayError } from "components/GlobalSnackbar/utils"
import { useProxy } from "contexts/ProxyContext"

export const WorkspaceProxyPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()

  const description =
    "Workspace proxies are used to reduce the latency of connections to a" +
    "workspace. To get the best experience, choose the workspace proxy that is" +
    "closest located to you."

  const { proxy, setProxy } = useProxy()

  const {
    data: proxiesResponse,
    error: getProxiesError,
    isFetching,
    isFetched,
  } = useWorkspaceProxiesData()

  return (
    <Section
      title="Workspace Proxies"
      className={styles.section}
      description={description}
      layout="fluid"
    >
      <WorkspaceProxyView
        proxies={proxiesResponse?.regions}
        isLoading={isFetching}
        hasLoaded={isFetched}
        getWorkspaceProxiesError={getProxiesError}
        preferredProxy={proxy.selectedProxy}
        onSelect={(proxy) => {
          if (!proxy.healthy) {
            displayError("Please select a healthy workspace proxy.")
            return
          }

          // Set the fetched regions + the selected proxy
          setProxy(proxiesResponse ? proxiesResponse.regions : [], proxy)
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
