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

export const ProxyRow: FC<{
  proxy: Region
  onSelectRegion: (proxy: Region) => void
  preferred: boolean
}> = ({ proxy, onSelectRegion, preferred }) => {
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
              <Avatar src={proxy.icon_url} variant="square" fitImage />
            )
          }
        />
      </TableCell>

      <TableCell>{proxy.path_app_url}</TableCell>
      <TableCell>
        <ProxyStatus proxy={proxy} />
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
    // TODO: What is the best way to show what proxy is currently being used?
    backgroundColor: theme.palette.secondary.main,
    outline: `3px solid ${theme.palette.secondary.light}`,
    outlineOffset: -3,
  },
}))
