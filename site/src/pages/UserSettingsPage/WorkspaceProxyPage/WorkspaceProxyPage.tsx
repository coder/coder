import { FC, PropsWithChildren, useState } from "react"
import { Section } from "components/SettingsLayout/Section"
import { WorkspaceProxyPageView } from "./WorkspaceProxyView"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { Trans } from "react-i18next"
import { useWorkspaceProxiesData } from "./hooks"
import { Region } from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
// import { ConfirmDeleteDialog } from "./components"
// import { Stack } from "components/Stack/Stack"
// import Button from "@material-ui/core/Button"
// import { Link as RouterLink } from "react-router-dom"
// import AddIcon from "@material-ui/icons/AddOutlined"
// import { APIKeyWithOwner } from "api/typesGenerated"

export const WorkspaceProxyPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()

  const description = (
    <Trans values={{}}>
      Workspace proxies are used to reduce the latency of connections to a workspace.
      To get the best experience, choose the workspace proxy that is closest located to
      you.
    </Trans>
  )

  const [preferred, setPreffered] = useState(getPreferredProxy())

  const {
    data: response,
    error: getProxiesError,
    isFetching,
    isFetched,
  } = useWorkspaceProxiesData()

  return (
    <>
      <Section
        title="Workspace Proxies"
        className={styles.section}
        description={description}
        layout="fluid"
      >
        <WorkspaceProxyPageView
          proxies={response ? response.regions : []}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getWorkspaceProxiesError={getProxiesError}
          preferredProxy={preferred}
          onSelect={(proxy) => {
            if (!proxy.healthy) {
              displayError("Please select a healthy workspace proxy.")
              return
            }
            // normProxy is a normalized proxy to 
            const normProxy = {
              ...proxy,
              // Trim trailing slashes to be consistent
              path_app_url: proxy.path_app_url.replace(/\/$/, ''),
            }

            savePreferredProxy(normProxy)
            setPreffered(normProxy)
          }}
        />
      </Section>
    </>
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


// Exporting to be used in the tests
export const savePreferredProxy = (proxy: Region): void => {
  window.localStorage.setItem("preferred-proxy", JSON.stringify(proxy))
}

export const getPreferredProxy = (): Region | undefined => {
  const str = localStorage.getItem("preferred-proxy")
  if (str === undefined || str === null) {
    return undefined
  }
  const proxy = JSON.parse(str)
  if (proxy.id === undefined || proxy.id === null) {
    return undefined
  }
  return proxy
}

export const clearPreferredProxy = (): void => {
  localStorage.removeItem("preferred-proxy")
}
