import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { DeploymentConfig } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { EnterpriseBadge } from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"

export type GitAuthSettingsPageViewProps = {
  deploymentConfig: Pick<DeploymentConfig, "gitauth">
}

export const GitAuthSettingsPageView = ({
  deploymentConfig,
}: GitAuthSettingsPageViewProps): JSX.Element => {
  const styles = useStyles()

  return (
    <>
      <Header
        title="Git Authentication"
        description="Coder integrates with GitHub, GitLab, BitBucket, and Azure Repos to authenticate developers with your Git provider."
        docsHref="https://coder.com/docs/coder-oss/latest/admin/git-providers"
      />

      <video
        autoPlay
        muted
        loop
        playsInline
        src="/gitauth.mp4"
        style={{
          maxWidth: "100%",
          borderRadius: 4,
        }}
      />

      <div className={styles.description}>
        <AlertBanner
          severity="info"
          text="Integrating with multiple Git providers is an Enterprise feature."
          actions={[<EnterpriseBadge key="enterprise" />]}
        />
      </div>

      <TableContainer>
        <Table className={styles.table}>
          <TableHead>
            <TableRow>
              <TableCell width="25%">Type</TableCell>
              <TableCell width="25%">Client ID</TableCell>
              <TableCell width="25%">Match</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {deploymentConfig.gitauth.value.length === 0 && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.empty}>
                    No providers have been configured!
                  </div>
                </TableCell>
              </TableRow>
            )}

            {deploymentConfig.gitauth.value.map((git) => {
              const name = git.id || git.type
              return (
                <TableRow key={name}>
                  <TableCell>{name}</TableCell>
                  <TableCell>{git.client_id}</TableCell>
                  <TableCell>{git.regex || "Not Set"}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  table: {
    "& td": {
      paddingTop: theme.spacing(3),
      paddingBottom: theme.spacing(3),
    },

    "& td:last-child, & th:last-child": {
      paddingLeft: theme.spacing(4),
    },
  },
  description: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
  },
  empty: {
    textAlign: "center",
  },
}))
