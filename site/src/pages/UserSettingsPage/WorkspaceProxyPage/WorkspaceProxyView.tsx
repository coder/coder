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
import IconButton from "@material-ui/core/IconButton/IconButton"
import { useTranslation } from "react-i18next"
import { Region } from "api/typesGenerated"
import CheckBoxOutlineBlankIcon from '@material-ui/icons/CheckBoxOutlineBlank';
import { Avatar } from "components/Avatar/Avatar"
import { AvatarData } from "components/AvatarData/AvatarData"
import { HealthyBadge, NotHealthyBadge } from "components/DeploySettingsLayout/Badges"



export interface WorkspaceProxyPageViewProps {
  proxies?: Region[]
  getWorkspaceProxiesError?: Error | unknown
  isLoading: boolean
  hasLoaded: boolean
  onSelect: (proxy: Region) => void
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
}) => {
    const theme = useTheme()
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
                <TableCell width="0%"></TableCell>
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
                  {proxies?.map((proxy) => {
                    return (
                      <TableRow
                        key={proxy.name}
                        data-testid={`${proxy.name}`}
                        tabIndex={0}
                      >
                        <TableCell>
                          <AvatarData
                            title={
                              proxy.display_name && proxy.display_name.length > 0
                                ? proxy.display_name
                                : proxy.name
                            }
                            // subtitle={proxy.description}
                            avatar={
                              proxy.icon_url !== "" && <Avatar src={proxy.icon_url} variant="square" fitImage />
                            }
                          />
                        </TableCell>

                        {/* <TableCell>
                          <span style={{ color: theme.palette.text.secondary }}>
                            {proxy.name}
                          </span>
                        </TableCell> */}

                        <TableCell>{proxy.path_app_url}</TableCell>
                        {/* <TableCell>{lastUsedOrNever(token.last_used)}</TableCell> */}
                        {/* <TableCell>{proxy.wildcard_hostname}</TableCell> */}
                        {/* <TableCell>
                          <span
                            style={{ color: theme.palette.text.secondary }}
                            data-chromatic="ignore"
                          >
                            {dayjs(token.expires_at).fromNow()}
                          </span>
                        </TableCell> */}
                        <TableCell><ProxyStatus proxy={proxy} /></TableCell>
                        {/* <TableCell>
                          <span style={{ color: theme.palette.text.secondary }}>
                            {dayjs(token.created_at).fromNow()}
                          </span>
                        </TableCell> */}

                        <TableCell>
                          <span style={{ color: theme.palette.text.secondary }}>
                            <IconButton
                              onClick={() => {
                                onSelect(proxy)
                              }}
                              size="medium"
                              aria-label={t("proxyActions.selectProxy.select")}
                            >
                              <CheckBoxOutlineBlankIcon />
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


export interface WorkspaceProxyStatusProps {
  proxy: Region
}

const ProxyStatus: FC<React.PropsWithChildren<WorkspaceProxyStatusProps>> = ({ proxy }) => {
  let icon = <NotHealthyBadge />
  if (proxy.healthy) {
    icon = <HealthyBadge />
  }

  return icon
}
