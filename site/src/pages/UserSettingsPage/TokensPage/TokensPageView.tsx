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
import { FC } from "react"

export const Language = {
  idLabel: "ID",
  createdAtLabel: "Created At",
  lastUsedLabel: "Last Used",
  expiresAtLabel: "Expires At",
  emptyMessage: "No tokens found",
}

export interface TokensPageViewProps {
  tokens?: APIKey[]
  getTokensError?: Error | unknown
}

export const TokensPageView: FC<
  React.PropsWithChildren<TokensPageViewProps>
> = ({
  tokens,
}) => {
  const theme = useTheme()

  return (
    <Stack>
      <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="34%">{Language.idLabel}</TableCell>
                  <TableCell width="16%">{Language.createdAtLabel}</TableCell>
                  <TableCell width="16%">{Language.lastUsedLabel}</TableCell>
                  <TableCell width="16%">{Language.expiresAtLabel}</TableCell>
                  <TableCell width="1%"></TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <Maybe condition={tokens === undefined}>
                  <TableLoader />
                </Maybe>

                <ChooseOne>
                  <Cond condition={Boolean(tokens?.length)}>
                    <TableEmpty
                      message={Language.emptyMessage}
                    />
                  </Cond>
                  <Cond>
                    {tokens?.map((token) => {

                      return (
                        <TableRow
                          key={token.id}
                          hover
                          data-testid={`token-${token.id}`}
                          tabIndex={0}
                        >
                          <TableCell>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {token.id}
                            </span>
                          </TableCell>

                          <TableCell>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {token.created_at}
                            </span>
                          </TableCell>


                          <TableCell>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {token.last_used}
                            </span>
                          </TableCell>

                          <TableCell>
                            <span
                              style={{ color: theme.palette.text.secondary }}
                            >
                              {token.expires_at}
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
