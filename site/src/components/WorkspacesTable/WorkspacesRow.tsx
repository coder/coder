import TableCell from "@mui/material/TableCell"
import { makeStyles } from "@mui/styles"
import TableRow from "@mui/material/TableRow"
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight"
import { AvatarData } from "components/AvatarData/AvatarData"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { getDisplayWorkspaceTemplateName } from "utils/workspace"
import { LastUsed } from "../LastUsed/LastUsed"
import { Workspace } from "api/typesGenerated"
import { OutdatedHelpTooltip } from "components/Tooltips/OutdatedHelpTooltip"
import { Avatar } from "components/Avatar/Avatar"
import { Stack } from "components/Stack/Stack"
import { useClickableTableRow } from "hooks/useClickableTableRow"

export const WorkspacesRow: FC<{
  workspace: Workspace
  onUpdateWorkspace: (workspace: Workspace) => void
}> = ({ workspace, onUpdateWorkspace }) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace)
  const clickable = useClickableTableRow(() => {
    navigate(workspacePageLink)
  })

  return (
    <TableRow data-testid={`workspace-${workspace.id}`} {...clickable}>
      <TableCell>
        <AvatarData
          title={
            <Stack direction="row" spacing={0} alignItems="center">
              {workspace.name}
              {workspace.outdated && (
                <OutdatedHelpTooltip
                  onUpdateVersion={() => {
                    onUpdateWorkspace(workspace)
                  }}
                />
              )}
            </Stack>
          }
          subtitle={workspace.owner_name}
          avatar={
            <Avatar
              src={workspace.template_icon}
              variant={workspace.template_icon ? "square" : undefined}
              fitImage={Boolean(workspace.template_icon)}
            >
              {workspace.name}
            </Avatar>
          }
        />
      </TableCell>

      <TableCell>{displayTemplateName}</TableCell>

      <TableCell>
        <LastUsed lastUsedAt={workspace.last_used_at} />
      </TableCell>

      <TableCell>
        <WorkspaceStatusBadge workspace={workspace} />
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
  arrowRight: {
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  },

  arrowCell: {
    display: "flex",
    paddingLeft: theme.spacing(2),
  },
}))
