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

export const ProxyRow: FC<{
  latency?: ProxyLatencyReport
  proxy: Region
}> = ({ proxy, latency }) => {
  // If we have a more specific proxy status, use that.
  // All users can see healthy/unhealthy, some can see more.
  let statusBadge = <ProxyStatus proxy={proxy} />
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
    </>
  )
}

// DetailedProxyStatus allows a more precise status to be displayed.
const DetailedProxyStatus: FC<{
  proxy: WorkspaceProxy
}> = ({ proxy }) => {
  if (!proxy.status) {
    // If the status is null/undefined/not provided, just go with the boolean "healthy" value.
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

// ProxyStatus will only show "healthy" or "not healthy" status.
const ProxyStatus: FC<{
  proxy: Region
}> = ({ proxy }) => {
  let icon = <NotHealthyBadge />
  if (proxy.healthy) {
    icon = <HealthyBadge />
  }

  return icon
}
