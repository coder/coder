import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import useTheme from "@material-ui/styles/useTheme"
import { useActor } from "@xstate/react"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { createDayString } from "util/createDayString"
import { getDisplayWorkspaceBuildInitiatedBy } from "util/workspace"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"
import { AvatarData } from "../AvatarData/AvatarData"
import {
  TableCellData,
  TableCellDataPrimary,
  TableCellDataSecondary,
} from "../TableCellData/TableCellData"
import { TableCellLink } from "../TableCellLink/TableCellLink"
import { OutdatedHelpTooltip } from "../Tooltips"

const Language = {
  upToDateLabel: "Up to date",
  outdatedLabel: "Outdated",
}

export const WorkspacesRow: FC<React.PropsWithChildren<{ workspaceRef: WorkspaceItemMachineRef }>> = ({ workspaceRef }) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const theme: Theme = useTheme()
  const [workspaceState, send] = useActor(workspaceRef)
  const { data: workspace } = workspaceState.context
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(workspace.latest_build)
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`

  return (
    <TableRow
      hover
      data-testid={`workspace-${workspace.id}`}
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          navigate(workspacePageLink)
        }
      }}
      className={styles.clickableTableRow}
    >
      <TableCellLink to={workspacePageLink}>
        <TableCellData>
          <TableCellDataPrimary highlight>{workspace.name}</TableCellDataPrimary>
          <TableCellDataSecondary>{workspace.owner_name}</TableCellDataSecondary>
        </TableCellData>
      </TableCellLink>

      <TableCellLink to={workspacePageLink}>{workspace.template_name}</TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <AvatarData
          title={initiatedBy}
          subtitle={createDayString(workspace.latest_build.created_at)}
        />
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        {workspace.outdated ? (
          <span className={styles.outdatedLabel}>
            {Language.outdatedLabel}
            <OutdatedHelpTooltip
              onUpdateVersion={() => {
                send("UPDATE_VERSION")
              }}
            />
          </span>
        ) : (
          <span style={{ color: theme.palette.text.secondary }}>{Language.upToDateLabel}</span>
        )}
      </TableCellLink>

      <TableCellLink to={workspacePageLink}>
        <WorkspaceStatusBadge build={workspace.latest_build} />
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <div className={styles.arrowCell}>
          <KeyboardArrowRight className={styles.arrowRight} />
        </div>
      </TableCellLink>
    </TableRow>
  )
}

const useStyles = makeStyles((theme) => ({
  clickableTableRow: {
    "&:hover td": {
      backgroundColor: fade(theme.palette.primary.dark, 0.1),
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
  outdatedLabel: {
    color: theme.palette.error.main,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },
  buildTime: {
    color: theme.palette.text.secondary,
    fontSize: 12,
  },
}))
