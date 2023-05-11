import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Stack } from "components/Stack/Stack"
import { TableEmpty } from "components/TableEmpty/TableEmpty"
import { TableLoader } from "components/TableLoader/TableLoader"
import { FC } from "react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Region } from "api/typesGenerated"
import { ProxyRow } from "./WorkspaceProxyRow"

export interface WorkspaceProxyViewProps {
  proxies?: Region[]
  getWorkspaceProxiesError?: Error | unknown
  isLoading: boolean
  hasLoaded: boolean
  onSelect: (proxy: Region) => void
  preferredProxy?: Region
  selectProxyError?: Error | unknown
}

export const WorkspaceProxyView: FC<
  React.PropsWithChildren<WorkspaceProxyViewProps>
> = ({
  proxies,
  getWorkspaceProxiesError,
  isLoading,
  hasLoaded,
  onSelect,
  selectProxyError,
  preferredProxy,
}) => {
  return (
    <Stack>
      {Boolean(getWorkspaceProxiesError) && (
        <AlertBanner severity="error" error={getWorkspaceProxiesError} />
      )}
      {Boolean(selectProxyError) && (
        <AlertBanner severity="error" error={selectProxyError} />
      )}
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell width="40%">Proxy</TableCell>
              <TableCell width="30%">URL</TableCell>
              <TableCell width="10%">Status</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={isLoading}>
                <TableLoader />
              </Cond>
              <Cond condition={hasLoaded && proxies?.length === 0}>
                <TableEmpty message="No workspace proxies found" />
              </Cond>
              <Cond>
                {proxies?.map((proxy) => (
                  <ProxyRow
                    key={proxy.id}
                    proxy={proxy}
                    onSelectRegion={onSelect}
                    preferred={
                      preferredProxy ? proxy.id === preferredProxy.id : false
                    }
                  />
                ))}
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  )
}
