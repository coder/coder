import { useTheme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { APIKey } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Maybe } from "components/Conditionals/Maybe"
import { Stack } from "components/Stack/Stack"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { TableLoader } from "components/TableLoader/TableLoader"
import DeleteOutlineIcon from "@material-ui/icons/DeleteOutline"
import dayjs from "dayjs"
import { FC } from "react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

import IconButton from "@material-ui/core/IconButton/IconButton"

export const Language = {
  idLabel: "ID",
  createdAtLabel: "Created At",
  lastUsedLabel: "Last Used",
  expiresAtLabel: "Expires At",
  emptyMessage: "No tokens found",
  ariaDeleteLabel: "Delete Token",
}

export interface TokensPageViewProps {
  tokens?: APIKey[]
  getTokensError?: Error | unknown
  isLoading: boolean
  hasLoaded: boolean
  onDelete: (id: string) => void
}

export const TokensPageView: FC<
  React.PropsWithChildren<TokensPageViewProps>
> = ({ tokens, getTokensError, isLoading, hasLoaded, onDelete }) => {
  const theme = useTheme()

  return (
    <Stack>
      {Boolean(getTokensError) && (
        <AlertBanner severity="error" error={getTokensError} />
      )}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="30%">{Language.idLabel}</TableCell>
              <TableCell width="20%">{Language.createdAtLabel}</TableCell>
              <TableCell width="20%">{Language.lastUsedLabel}</TableCell>
              <TableCell width="20%">{Language.expiresAtLabel}</TableCell>
              <TableCell width="10%"></TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <Maybe condition={isLoading}>
              <TableLoader />
            </Maybe>

            <ChooseOne>
              <Cond condition={hasLoaded && tokens?.length === 0}>
                <TableEmpty message={Language.emptyMessage} />
              </Cond>
              <Cond>
                {tokens?.map((token) => {
                  const t = dayjs(token.last_used)
                  const now = dayjs()
                  const lastUsed = now.isBefore(t.add(100, "year"))
                    ? t.fromNow()
                    : "Never"
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

                      <TableCell>{lastUsed}</TableCell>

                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          {dayjs(token.expires_at).fromNow()}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span style={{ color: theme.palette.text.secondary }}>
                          <IconButton
                            onClick={() => {
                              onDelete(token.id)
                            }}
                            size="medium"
                            aria-label={Language.ariaDeleteLabel}
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
