import { useTheme } from "@mui/styles";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import dayjs from "dayjs";
import { FC } from "react";
import IconButton from "@mui/material/IconButton/IconButton";
import { useTranslation } from "react-i18next";
import { APIKeyWithOwner } from "api/typesGenerated";
import relativeTime from "dayjs/plugin/relativeTime";
import { ErrorAlert } from "components/Alert/ErrorAlert";

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
}

export const TokensPageView: FC<
  React.PropsWithChildren<TokensPageViewProps>
> = ({
  tokens,
  getTokensError,
  isLoading,
  hasLoaded,
  onDelete,
  deleteTokenError,
}) => {
  const theme = useTheme();
  const { t } = useTranslation("tokensPage");

  return (
    <Stack>
      {Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
      {Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="20%">{t("table.id")}</TableCell>
              <TableCell width="20%">{t("table.name")}</TableCell>
              <TableCell width="20%">{t("table.lastUsed")}</TableCell>
              <TableCell width="20%">{t("table.expiresAt")}</TableCell>
              <TableCell width="20%">{t("table.createdAt")}</TableCell>
              <TableCell width="0%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={isLoading}>
                <TableLoader />
              </Cond>
              <Cond condition={hasLoaded && tokens?.length === 0}>
                <TableEmpty message={t("emptyState")} />
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
                            aria-label={t(
                              "tokenActions.deleteToken.delete",
                            ).toString()}
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
