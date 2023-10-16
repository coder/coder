import { makeStyles } from "@mui/styles";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { DeploymentValues, ExternalAuthConfig } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { EnterpriseBadge } from "components/DeploySettingsLayout/Badges";
import { Header } from "components/DeploySettingsLayout/Header";
import { docs } from "utils/docs";

export type ExternalAuthSettingsPageViewProps = {
  config: DeploymentValues;
};

export const ExternalAuthSettingsPageView = ({
  config,
}: ExternalAuthSettingsPageViewProps): JSX.Element => {
  const styles = useStyles();

  return (
    <>
      <Header
        title="External Authentication"
        description="Coder integrates with GitHub, GitLab, BitBucket, Azure Repos, and OpenID Connect to authenticate developers with external services."
        docsHref={docs("/admin/external-auth")}
      />

      <video
        autoPlay
        muted
        loop
        playsInline
        src="/external-auth.mp4"
        style={{
          maxWidth: "100%",
          borderRadius: 4,
        }}
      />

      <div className={styles.description}>
        <Alert severity="info" actions={<EnterpriseBadge key="enterprise" />}>
          Integrating with multiple External authentication providers is an
          Enterprise feature.
        </Alert>
      </div>

      <TableContainer>
        <Table className={styles.table}>
          <TableHead>
            <TableRow>
              <TableCell width="25%">ID</TableCell>
              <TableCell width="25%">Client ID</TableCell>
              <TableCell width="25%">Match</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {((config.external_auth === null ||
              config.external_auth?.length === 0) && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.empty}>
                    No providers have been configured!
                  </div>
                </TableCell>
              </TableRow>
            )) ||
              config.external_auth?.map((git: ExternalAuthConfig) => {
                const name = git.id || git.type;
                return (
                  <TableRow key={name}>
                    <TableCell>{name}</TableCell>
                    <TableCell>{git.client_id}</TableCell>
                    <TableCell>{git.regex || "Not Set"}</TableCell>
                  </TableRow>
                );
              })}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

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
}));
