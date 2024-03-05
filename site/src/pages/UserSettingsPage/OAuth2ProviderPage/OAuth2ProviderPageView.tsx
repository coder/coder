import Button from "@mui/material/Button";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { TableLoader } from "components/TableLoader/TableLoader";

export type OAuth2ProviderPageViewProps = {
  isLoading: boolean;
  error: unknown;
  apps?: TypesGen.OAuth2ProviderApp[];
  revoke: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2ProviderPageView: FC<OAuth2ProviderPageViewProps> = ({
  isLoading,
  error,
  apps,
  revoke,
}) => {
  return (
    <>
      {error && <ErrorAlert error={error} />}

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="100%">Name</TableCell>
              <TableCell width="1%" />
            </TableRow>
          </TableHead>
          <TableBody>
            {isLoading && <TableLoader />}
            {apps?.map((app) => (
              <OAuth2AppRow key={app.id} app={app} revoke={revoke} />
            ))}
            {apps?.length === 0 && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div css={{ textAlign: "center" }}>
                    No OAuth2 applications have been authorized.
                  </div>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

type OAuth2AppRowProps = {
  app: TypesGen.OAuth2ProviderApp;
  revoke: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2AppRow: FC<OAuth2AppRowProps> = ({ app, revoke }) => {
  return (
    <TableRow key={app.id} data-testid={`app-${app.id}`}>
      <TableCell>
        <AvatarData
          title={app.name}
          avatar={
            Boolean(app.icon) && (
              <Avatar src={app.icon} variant="square" fitImage />
            )
          }
        />
      </TableCell>

      <TableCell>
        <Button
          variant="contained"
          size="small"
          color="error"
          onClick={() => revoke(app)}
        >
          Revoke&hellip;
        </Button>
      </TableCell>
    </TableRow>
  );
};

export default OAuth2ProviderPageView;
