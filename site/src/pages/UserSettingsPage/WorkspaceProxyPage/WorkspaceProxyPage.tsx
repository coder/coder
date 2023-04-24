import { FC, PropsWithChildren } from "react"
import { Section } from "components/SettingsLayout/Section"
import { WorkspaceProxyPageView } from "./WorkspaceProxyView"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation, Trans } from "react-i18next"
import { useWorkspaceProxiesData } from "./hooks"
// import { ConfirmDeleteDialog } from "./components"
// import { Stack } from "components/Stack/Stack"
// import Button from "@material-ui/core/Button"
// import { Link as RouterLink } from "react-router-dom"
// import AddIcon from "@material-ui/icons/AddOutlined"
// import { APIKeyWithOwner } from "api/typesGenerated"

export const WorkspaceProxyPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  const { t } = useTranslation("workspaceProxyPage")

  const description = (
    <Trans t={t} i18nKey="description" values={{}}>
      Workspace proxies are used to reduce the latency of connections to a workspace.
      To get the best experience, choose the workspace proxy that is closest located to
      you.
    </Trans>
  )

  // const [tokenToDelete, setTokenToDelete] = useState<
  //   APIKeyWithOwner | undefined
  // >(undefined)

  const {
    data: proxies,
    error: getProxiesError,
    isFetching,
    isFetched,
  } = useWorkspaceProxiesData()

  return (
    <>
      <Section
        title={t("title")}
        className={styles.section}
        description={description}
        layout="fluid"
      >
        <WorkspaceProxyPageView
          proxies={proxies}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getWorkspaceProxiesError={getProxiesError}
          onSelect={(proxy) => {
            console.log("selected", proxy)
          }}
        />
      </Section>
      {/* <ConfirmDeleteDialog
        queryKey={queryKey}
        token={tokenToDelete}
        setToken={setTokenToDelete}
      /> */}
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
  tokenActions: {
    marginBottom: theme.spacing(1),
  },
}))

export default WorkspaceProxyPage
