import { Region, WorkspaceProxy } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { Avatar } from "components/Avatar/Avatar"
import TableCell from "@mui/material/TableCell"
import TableRow from "@mui/material/TableRow"
import { FC } from "react"
import {
  HealthyBadge,
  NotHealthyBadge,
  NotReachableBadge,
  NotRegisteredBadge,
} from "components/DeploySettingsLayout/Badges"
import { ProxyLatencyReport } from "contexts/useProxyLatency"
import { getLatencyColor } from "utils/latency"
import { makeStyles } from "@mui/styles"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

export const ProxyRow: FC<{
  latency?: ProxyLatencyReport
  proxy: Region
}> = ({ proxy, latency }) => {
  const styles = useStyles()

  // If we have a more specific proxy status, use that.
  // All users can see healthy/unhealthy, some can see more.
  let statusBadge = <ProxyStatus proxy={proxy} />
  let proxyText
  if ("status" in proxy) {
    statusBadge = <DetailedProxyStatus proxy={proxy as WorkspaceProxy} />
  }

  return (
    <>
      <TableRow key={proxy.name} data-testid={`${proxy.name}`}>
        <TableCell>
          <AvatarData
            title={
              proxy.display_name && proxy.display_name.length > 0
                ? proxy.display_name
                : proxy.name
            }
            avatar={
              proxy.icon_url !== "" && (
                <Avatar
                  size="sm"
                  src={proxy.icon_url}
                  variant="square"
                  fitImage
                />
              )
            }
          />
        </TableCell>

        <TableCell sx={{ fontSize: 14 }}>{proxy.path_app_url}</TableCell>
        <TableCell sx={{ fontSize: 14 }}>
          {statusBadge}
          {/* <ProxyStatus proxy={proxy} /> */}
        </TableCell>
        <TableCell
          sx={{
            fontSize: 14,
            textAlign: "right",
            color: (theme) =>
              latency
                ? getLatencyColor(theme, latency.latencyMS)
                : theme.palette.text.secondary,
          }}
        >
          {latency ? `${latency.latencyMS.toFixed(0)} ms` : "Not available"}
        </TableCell>
      </TableRow>
      {proxyText ? (
        <TableRow className={styles.noBottomBorder}>
          <TableCell colSpan={4}>{proxyText}</TableCell>
        </TableRow>
      ) : null}
    </>
  )
}

const DetailedProxyStatus: FC<{
  proxy: WorkspaceProxy
}> = ({ proxy }) => {
  if (!proxy.status) {
    // If the status is not set, go with the less detailed version.
    return <ProxyStatus proxy={proxy} />
  }

  switch (proxy.status.status) {
    case "ok":
      return <HealthyBadge />
    case "unhealthy":
      return <NotHealthyBadge />
    case "unreachable":
      return <NotReachableBadge />
    case "unregistered":
      return <NotRegisteredBadge />
    default:
      return <NotHealthyBadge />
  }
}

const ProxyStatus: FC<{
  proxy: Region
}> = ({ proxy }) => {
  let icon = <NotHealthyBadge />
  if (proxy.healthy) {
    icon = <HealthyBadge />
  }

  return icon
}

const useStyles = makeStyles({
  noBottomBorder: {
    borderBottom: "none",
  },
  proxyStatusText: {
    // border: `1px solid ${theme.palette.success.light}`,
    // backgroundColor: theme.palette.success.dark,
    // textTransform: "none",
    // color: "white",
    fontFamily: MONOSPACE_FONT_FAMILY,
    textDecoration: "none",
  },
  proxyStatusContainer: {
    gap: "0px",
  },
})
