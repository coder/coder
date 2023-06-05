import { Region } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { Avatar } from "components/Avatar/Avatar"
import { useClickableTableRow } from "hooks/useClickableTableRow"
import TableCell from "@mui/material/TableCell"
import TableRow from "@mui/material/TableRow"
import { FC } from "react"
import {
  HealthyBadge,
  NotHealthyBadge,
} from "components/DeploySettingsLayout/Badges"
import { makeStyles } from "@mui/styles"
import { combineClasses } from "utils/combineClasses"
import { ProxyLatencyReport } from "contexts/useProxyLatency"
import { getLatencyColor } from "utils/colors"
import { alpha } from "@mui/material/styles"

export const ProxyRow: FC<{
  latency?: ProxyLatencyReport
  proxy: Region
  onSelectRegion: (proxy: Region) => void
  preferred: boolean
}> = ({ proxy, onSelectRegion, preferred, latency }) => {
  const styles = useStyles()

  const clickable = useClickableTableRow(() => {
    onSelectRegion(proxy)
  })

  return (
    <TableRow
      key={proxy.name}
      data-testid={`${proxy.name}`}
      {...clickable}
      // Make sure to include our classname here.
      className={combineClasses({
        [clickable.className]: true,
        [styles.preferredrow]: preferred,
      })}
    >
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
        <ProxyStatus proxy={proxy} />
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
  )
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

const useStyles = makeStyles((theme) => ({
  preferredrow: {
    backgroundColor: alpha(
      theme.palette.primary.main,
      theme.palette.action.hoverOpacity,
    ),
    outline: `1px solid ${theme.palette.primary.main}`,
    outlineOffset: "-1px",
  },
}))
