import TableCell from "@material-ui/core/TableCell"
import { makeStyles } from "@material-ui/core/styles"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import { AvatarData } from "components/AvatarData/AvatarData"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { useClickable } from "hooks/useClickable"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { getDisplayWorkspaceTemplateName } from "util/workspace"
import { LastUsed } from "../LastUsed/LastUsed"
import { Workspace } from "api/typesGenerated"
import { OutdatedHelpTooltip } from "components/Tooltips/OutdatedHelpTooltip"
import { Avatar } from "components/Avatar/Avatar"

export const WorkspacesRow: FC<{
  workspace: Workspace
  onUpdateWorkspace: (workspace: Workspace) => void
}> = ({ workspace, onUpdateWorkspace }) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`
  const hasTemplateIcon =
    workspace.template_icon && workspace.template_icon !== ""
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace)
  const clickable = useClickable(() => {
    navigate(workspacePageLink)
  })

  return (
    <TableRow
      className={styles.row}
      hover
      data-testid={`workspace-${workspace.id}`}
      {...clickable}
    >
      <TableCell>
        <AvatarData
          title={workspace.name}
          subtitle={workspace.owner_name}
          avatar={
            hasTemplateIcon && (
              <Avatar src={workspace.template_icon} variant="square" fitImage />
            )
          }
        />
      </TableCell>

      <TableCell>{displayTemplateName}</TableCell>

      <TableCell>
        <div className={styles.version}>
          {workspace.latest_build.template_version_name}
          {workspace.outdated && (
            <OutdatedHelpTooltip
              onUpdateVersion={() => {
                onUpdateWorkspace(workspace)
              }}
            />
          )}
        </div>
      </TableCell>

      <TableCell>
        <LastUsed lastUsedAt={workspace.last_used_at} />
      </TableCell>

      <TableCell>
        <WorkspaceStatusBadge build={workspace.latest_build} />
      </TableCell>

      <TableCell>
        <div className={styles.arrowCell}>
          <KeyboardArrowRight className={styles.arrowRight} />
        </div>
      </TableCell>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  row: {
    cursor: "pointer",

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
      outlineOffset: -1,
    },
  },

  arrowRight: {
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  },

  arrowCell: {
    display: "flex",
    paddingLeft: theme.spacing(2),
  },

  templateIconWrapper: {
    // Same size then the avatar component
    width: 36,
    height: 36,
    padding: 2,

    "& img": {
      width: "100%",
    },
  },

  version: {
    display: "flex",
  },
}))
