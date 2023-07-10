import { Region, WorkspaceProxy } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import { Avatar } from "components/Avatar/Avatar"
import TableCell from "@mui/material/TableCell"
import TableRow from "@mui/material/TableRow"
import { FC, useState } from "react"
import {
  HealthyBadge,
  NotHealthyBadge,
  NotReachableBadge,
  NotRegisteredBadge,
} from "components/DeploySettingsLayout/Badges"
import { ProxyLatencyReport } from "contexts/useProxyLatency"
import { getLatencyColor } from "utils/latency"
import Collapse from "@mui/material/Collapse"
import { makeStyles } from "@mui/styles"
import { combineClasses } from "utils/combineClasses"
import ListItem from "@mui/material/ListItem"
import List from "@mui/material/List"
import ListSubheader from "@mui/material/ListSubheader"
import { Maybe } from "components/Conditionals/Maybe"
import { CodeExample } from "components/CodeExample/CodeExample"

export const ProxyRow: FC<{
  latency?: ProxyLatencyReport
  proxy: Region
}> = ({ proxy, latency }) => {
  const styles = useStyles()
  // If we have a more specific proxy status, use that.
  // All users can see healthy/unhealthy, some can see more.
  let statusBadge = <ProxyStatus proxy={proxy} />
  let shouldShowMessages = false
  if ("status" in proxy) {
    const wsproxy = proxy as WorkspaceProxy
    statusBadge = <DetailedProxyStatus proxy={wsproxy} />
    shouldShowMessages = Boolean(
      (wsproxy.status?.report?.warnings &&
        wsproxy.status?.report?.warnings.length > 0) ||
        (wsproxy.status?.report?.errors &&
          wsproxy.status?.report?.errors.length > 0),
    )
  }

  const [isMsgsOpen, setIsMsgsOpen] = useState(false)
  const toggle = () => {
    if (shouldShowMessages) {
      setIsMsgsOpen((v) => !v)
    }
  }
  return (
    <>
      <TableRow
        className={combineClasses({
          [styles.clickable]: shouldShowMessages,
        })}
        key={proxy.name}
        data-testid={`${proxy.name}`}
      >
        <TableCell onClick={toggle}>
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
            subtitle={shouldShowMessages ? "Click to view details" : undefined}
          />
        </TableCell>

        <TableCell sx={{ fontSize: 14 }}>{proxy.path_app_url}</TableCell>
        <TableCell sx={{ fontSize: 14 }}>{statusBadge}</TableCell>
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
      <Maybe condition={shouldShowMessages}>
        <TableRow>
          <TableCell colSpan={4} sx={{ padding: "0px !important" }}>
            <Collapse in={isMsgsOpen}>
              <ProxyMessagesRow proxy={proxy as WorkspaceProxy} />
            </Collapse>
          </TableCell>
        </TableRow>
      </Maybe>
    </>
  )
}

const ProxyMessagesRow: FC<{
  proxy: WorkspaceProxy
}> = ({ proxy }) => {
  return (
    <>
      <ProxyMessagesList
        title="Errors"
        messages={proxy.status?.report?.errors}
      />
      <ProxyMessagesList
        title="Warnings"
        messages={proxy.status?.report?.warnings}
      />
    </>
  )
}

const ProxyMessagesList: FC<{
  title: string
  messages?: string[]
}> = ({ title, messages }) => {
  if (!messages) {
    return <></>
  }
  return (
    <List
      subheader={
        <ListSubheader component="div" id="nested-list-subheader">
          {title}
        </ListSubheader>
      }
    >
      {messages.map((error, index) => (
        <ListItem key={"message" + index}>
          <CodeExample code={error} />
        </ListItem>
      ))}
    </List>
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

const useStyles = makeStyles((theme) => ({
  clickable: {
    cursor: "pointer",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },
  },
}))
