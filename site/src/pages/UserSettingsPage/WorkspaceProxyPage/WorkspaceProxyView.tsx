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
import { FC } from "react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"
import { Region } from "api/typesGenerated"
import { ProxyRow } from "./WorkspaceProxyRow"



export interface WorkspaceProxyPageViewProps {
  proxies?: Region[]
  getWorkspaceProxiesError?: Error | unknown
  isLoading: boolean
  hasLoaded: boolean
  onSelect: (proxy: Region) => void
  preferredProxy?: Region
  selectProxyError?: Error | unknown
}

export const WorkspaceProxyPageView: FC<
  React.PropsWithChildren<WorkspaceProxyPageViewProps>
> = ({
  proxies,
  getWorkspaceProxiesError,
  isLoading,
  hasLoaded,
  onSelect,
  selectProxyError,
  preferredProxy,
}) => {
    const { t } = useTranslation("proxyPage")

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
                <TableCell width="40%">{t("table.icon")}</TableCell>
                <TableCell width="30%">{t("table.url")}</TableCell>
                <TableCell width="10%">{t("table.status")}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              <ChooseOne>
                <Cond condition={isLoading}>
                  <TableLoader />
                </Cond>
                <Cond condition={hasLoaded && proxies?.length === 0}>
                  <TableEmpty message={t("emptyState")} />
                </Cond>
                <Cond>
                  {proxies?.map((proxy) => (
                    <ProxyRow
                      key={proxy.id}
                      proxy={proxy}
                      onSelectRegion={onSelect}
                      preferred={preferredProxy ? proxy.id === preferredProxy.id : false}
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
