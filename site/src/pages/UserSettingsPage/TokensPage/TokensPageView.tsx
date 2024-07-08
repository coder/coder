import { useTheme } from "@emotion/react";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import IconButton from "@mui/material/IconButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import type { FC, ReactNode } from "react";
import type { APIKeyWithOwner } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";

dayjs.extend(relativeTime);

const lastUsedOrNever = (lastUsed: string) => {
  const t = dayjs(lastUsed);
  const now = dayjs();
  return now.isBefore(t.add(100, "year")) ? t.fromNow() : "Never";
};

export interface TokensPageViewProps {
  tokens?: APIKeyWithOwner[];
  getTokensError?: unknown;
  isLoading: boolean;
  hasLoaded: boolean;
  onDelete: (token: APIKeyWithOwner) => void;
  deleteTokenError?: unknown;
  children?: ReactNode;
}

export const TokensPageView: FC<TokensPageViewProps> = ({
  tokens,
  getTokensError,
  isLoading,
  hasLoaded,
  onDelete,
  deleteTokenError,
}) => {
  const theme = useTheme();

  return (
    <Stack>
      {Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
      {Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="20%">ID</TableCell>
              <TableCell width="20%">Name</TableCell>
              <TableCell width="20%">Last Used</TableCell>
              <TableCell width="20%">Expires At</TableCell>
              <TableCell width="20%">Created At</TableCell>
              <TableCell width="0%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={isLoading}>
                <TableLoader />
              </Cond>
              <Cond condition={hasLoaded && (!tokens || tokens.length === 0)}>
                <TableEmpty message="No tokens found" />
              </Cond>
              <Cond>
                {tokens?.map((token) => {
                  return (
                    <TableRow
                      key={token.id}
                      data-testid={`token-${token.id}`}
                      tabIndex={0}
                    >
                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          {token.id}
                        </span>
                      </TableCell>

                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          {token.token_name}
                        </span>
                      </TableCell>

                      <TableCell>{lastUsedOrNever(token.last_used)}</TableCell>

                      <TableCell>
                        <span
                          style={{ color: theme.palette.text.secondary }}
                          data-chromatic="ignore"
                        >
                          {dayjs(token.expires_at).fromNow()}
                        </span>
                      </TableCell>

                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          {dayjs(token.created_at).fromNow()}
                        </span>
                      </TableCell>

                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          <IconButton
                            onClick={() => {
                              onDelete(token);
                            }}
                            size="medium"
                            aria-label="Delete token"
                          >
                            <DeleteOutlineIcon />
                          </IconButton>
                        </span>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  );
};
