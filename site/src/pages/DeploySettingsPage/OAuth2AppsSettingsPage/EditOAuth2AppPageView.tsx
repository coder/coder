import { useTheme } from "@emotion/react";
import Divider from "@mui/material/Divider";
import LoadingButton from "@mui/lab/LoadingButton";
import Button from "@mui/material/Button";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft";
import { type FC, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { CodeExample } from "components/CodeExample/CodeExample";
import { CopyableValue } from "components/CopyableValue/CopyableValue";
import { Header } from "components/DeploySettingsLayout/Header";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { TableLoader } from "components/TableLoader/TableLoader";
import { createDayString } from "utils/createDayString";
import { OAuth2AppForm } from "./OAuth2AppForm";

type EditOAuth2AppProps = {
  app?: TypesGen.OAuth2App;
  isLoadingApp: boolean;
  isLoadingSecrets: boolean;
  isUpdating:
    | "update-app"
    | "create-secret"
    | "delete-app"
    | "delete-secret"
    | false;
  updateApp: (req: TypesGen.PutOAuth2AppRequest) => void;
  deleteApp: () => void;
  generateAppSecret: () => void;
  deleteAppSecret: (id: string) => void;
  secrets?: TypesGen.OAuth2AppSecret[];
  newAppSecret?: TypesGen.OAuth2AppSecretFull;
  dismissNewSecret: () => void;
  error?: unknown;
};

export const EditOAuth2AppPageView: FC<EditOAuth2AppProps> = ({
  app,
  isLoadingApp,
  isLoadingSecrets,
  isUpdating,
  updateApp,
  deleteApp,
  generateAppSecret,
  deleteAppSecret,
  secrets,
  newAppSecret,
  dismissNewSecret,
  error,
}) => {
  const theme = useTheme();
  const [searchParams] = useSearchParams();
  const [showDelete, setShowDelete] = useState<boolean>(false);

  return (
    <>
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <Header
          title="Edit OAuth2 application"
          description="Configure an application to use Coder as an OAuth2 provider."
        />
        <Button
          component={Link}
          startIcon={<KeyboardArrowLeft />}
          to="/deployment/oauth2-apps"
        >
          All OAuth2 Applications
        </Button>
      </Stack>

      {newAppSecret && (
        <ConfirmDialog
          hideCancel
          open={Boolean(newAppSecret)}
          onConfirm={() => dismissNewSecret()}
          onClose={() => dismissNewSecret()}
          title="OAuth2 client secret"
          confirmText="OK"
          description={
            <>
              <p>
                Your new client secret is displayed below. Make sure to copy it
                now; you will not be able to see it again.
              </p>
              <CodeExample
                code={newAppSecret.client_secret_full}
                css={{
                  minHeight: "auto",
                  userSelect: "all",
                  width: "100%",
                  marginTop: 24,
                }}
              />
            </>
          }
        />
      )}

      <Stack>
        {searchParams.has("created") && (
          <Alert severity="info" dismissible>
            Your OAuth2 application has been created. Generate a client secret
            below to start using your application.
          </Alert>
        )}

        {error ? <ErrorAlert error={error} /> : undefined}

        {isLoadingApp && <Loader />}

        {!isLoadingApp && app && (
          <>
            <DeleteDialog
              isOpen={showDelete}
              confirmLoading={isUpdating === "delete-app"}
              name={app.name}
              entity="OAuth2 application"
              onConfirm={() => deleteApp()}
              onCancel={() => setShowDelete(false)}
            />

            <Stack direction="row">
              <div>
                <strong>Client ID:</strong>
              </div>
              <CopyableValue value={app.id}>{app.id}</CopyableValue>
            </Stack>

            <Divider css={{ borderColor: theme.palette.divider }} />

            <OAuth2AppForm
              app={app}
              onSubmit={updateApp}
              isUpdating={isUpdating === "update-app"}
              error={error}
              actions={
                <Button
                  variant="outlined"
                  type="button"
                  color="error"
                  onClick={() => setShowDelete(true)}
                >
                  Delete&hellip;
                </Button>
              }
            />

            <Divider css={{ borderColor: theme.palette.divider }} />

            <OAuth2AppSecretsTable
              secrets={secrets}
              generateAppSecret={generateAppSecret}
              deleteAppSecret={deleteAppSecret}
              isLoadingSecrets={isLoadingSecrets}
              isUpdating={isUpdating}
            />
          </>
        )}
      </Stack>
    </>
  );
};

type OAuth2AppSecretsTableProps = {
  secrets?: TypesGen.OAuth2AppSecret[];
  generateAppSecret: () => void;
  isLoadingSecrets: boolean;
  isUpdating:
    | "update-app"
    | "create-secret"
    | "delete-app"
    | "delete-secret"
    | false;
  deleteAppSecret: (id: string) => void;
};

const OAuth2AppSecretsTable: FC<OAuth2AppSecretsTableProps> = ({
  secrets,
  generateAppSecret,
  isLoadingSecrets,
  isUpdating,
  deleteAppSecret,
}) => {
  return (
    <>
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <h2>Client secrets</h2>
        <LoadingButton
          loading={isUpdating === "create-secret"}
          type="submit"
          variant="contained"
          onClick={() => generateAppSecret()}
        >
          Generate secret
        </LoadingButton>
      </Stack>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="80%">Secret</TableCell>
              <TableCell width="20%">Last Used</TableCell>
              <TableCell width="1%" />
            </TableRow>
          </TableHead>
          <TableBody>
            {isLoadingSecrets && <TableLoader />}
            {!isLoadingSecrets && (!secrets || secrets?.length === 0) && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div css={{ textAlign: "center" }}>
                    No client secrets have been generated.
                  </div>
                </TableCell>
              </TableRow>
            )}
            {!isLoadingSecrets &&
              secrets &&
              secrets?.length > 0 &&
              secrets?.map((secret) => (
                <OAuth2SecretRow
                  key={secret.id}
                  secret={secret}
                  isUpdating={isUpdating}
                  deleteAppSecret={deleteAppSecret}
                />
              ))}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

type OAuth2SecretRowProps = {
  secret: TypesGen.OAuth2AppSecret;
  deleteAppSecret: (id: string) => void;
  isUpdating:
    | "update-app"
    | "create-secret"
    | "delete-app"
    | "delete-secret"
    | false;
};

const OAuth2SecretRow: FC<OAuth2SecretRowProps> = ({
  secret,
  deleteAppSecret,
  isUpdating,
}) => {
  const [showDelete, setShowDelete] = useState<boolean>(false);

  return (
    <TableRow key={secret.id} data-testid={`secret-${secret.id}`}>
      <TableCell>*****{secret.client_secret_truncated}</TableCell>
      <TableCell data-chromatic="ignore">
        {secret.last_used_at ? createDayString(secret.last_used_at) : "never"}
      </TableCell>
      <TableCell>
        <ConfirmDialog
          type="delete"
          hideCancel={false}
          open={showDelete}
          onConfirm={() => deleteAppSecret(secret.id)}
          onClose={() => setShowDelete(false)}
          title="Delete OAuth2 client secret"
          confirmLoading={isUpdating === "delete-secret"}
          confirmText="Delete"
          description={
            <>
              Deleting <strong>*****{secret.client_secret_truncated}</strong> is
              irreversible and will revoke all the tokens generated by it. Are
              you sure you want to proceed?
            </>
          }
        />
        <Button
          variant="outlined"
          type="button"
          color="error"
          onClick={() => setShowDelete(true)}
        >
          Delete&hellip;
        </Button>
      </TableCell>
    </TableRow>
  );
};
