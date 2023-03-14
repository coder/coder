import { useTheme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Stack } from "components/Stack/Stack"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { TableLoader } from "components/TableLoader/TableLoader"
import DeleteOutlineIcon from "@material-ui/icons/DeleteOutline"
import dayjs from "dayjs"
import { FC } from "react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import IconButton from "@material-ui/core/IconButton/IconButton"
import { useTranslation } from "react-i18next"
import { APIKeyWithOwner } from "api/typesGenerated"

const lastUsedOrNever = (lastUsed: string) => {
  const t = dayjs(lastUsed)
  const now = dayjs()
  return now.isBefore(t.add(100, "year")) ? t.fromNow() : "Never"
}

export interface TokensPageViewProps {
  tokens?: APIKeyWithOwner[]
  viewAllTokens: boolean
  getTokensError?: Error | unknown
  isLoading: boolean
  hasLoaded: boolean
  onDelete: (id: string) => void
  deleteTokenError?: Error | unknown
}

export const TokensPageView: FC<
  React.PropsWithChildren<TokensPageViewProps>
> = ({
  tokens,
  viewAllTokens,
  getTokensError,
  isLoading,
  hasLoaded,
  onDelete,
  deleteTokenError,
}) => {
  const theme = useTheme()
  const { t } = useTranslation("tokensPage")
  const colWidth = viewAllTokens ? "20%" : "25%"

  return (
    <Stack>
      {Boolean(getTokensError) && (
        <AlertBanner severity="error" error={getTokensError} />
      )}
      {Boolean(deleteTokenError) && (
        <AlertBanner severity="error" error={deleteTokenError} />
      )}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width={colWidth}>{t("table.id")}</TableCell>
              <TableCell width={colWidth}>{t("table.createdAt")}</TableCell>
              <TableCell width={colWidth}>{t("table.lastUsed")}</TableCell>
              <TableCell width={colWidth}>{t("table.expiresAt")}</TableCell>
              {viewAllTokens && (
                <TableCell width="20%">{t("table.owner")}</TableCell>
              )}
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
                          {dayjs(token.created_at).fromNow()}
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
                      {viewAllTokens && (
                        <TableCell>
                          <span style={{ color: theme.palette.text.secondary }}>
                            {token.username}
                          </span>
                        </TableCell>
                      )}
                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          <IconButton
                            onClick={() => {
                              onDelete(token.id)
                            }}
                            size="medium"
                            aria-label={t("deleteToken.delete")}
                          >
                            <DeleteOutlineIcon />
                          </IconButton>
                        </span>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  )
}
