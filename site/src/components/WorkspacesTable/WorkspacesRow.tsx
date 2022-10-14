import { makeStyles, Theme } from "@material-ui/core/styles"
import TableRow from "@material-ui/core/TableRow"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import useTheme from "@material-ui/styles/useTheme"
import { useActor } from "@xstate/react"
import { AvatarData } from "components/AvatarData/AvatarData"
import { WorkspaceStatusBadge } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge"
import { FC } from "react"
import { useNavigate } from "react-router-dom"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"
import { LastUsed } from "../LastUsed/LastUsed"
import {
  TableCellData,
  TableCellDataPrimary,
} from "../TableCellData/TableCellData"
import { TableCellLink } from "../TableCellLink/TableCellLink"
import { OutdatedHelpTooltip } from "../Tooltips"

const Language = {
  upToDateLabel: "Up to date",
  outdatedLabel: "Outdated",
}

export const WorkspacesRow: FC<
  React.PropsWithChildren<{ workspaceRef: WorkspaceItemMachineRef }>
> = ({ workspaceRef }) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const theme: Theme = useTheme()
  const [workspaceState, send] = useActor(workspaceRef)
  const { data: workspace } = workspaceState.context
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`
  const hasTemplateIcon =
    workspace.template_icon && workspace.template_icon !== ""

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
        <AvatarData
          highlightTitle
          title={workspace.name}
          subtitle={workspace.owner_name}
          avatar={
            hasTemplateIcon ? (
              <div className={styles.templateIconWrapper}>
                <img alt="" src={workspace.template_icon} />
              </div>
            ) : undefined
          }
        />
      </TableCellLink>

      <TableCellLink to={workspacePageLink}>
        <TableCellDataPrimary>{workspace.template_name}</TableCellDataPrimary>
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <TableCellData>
          <LastUsed lastUsedAt={workspace.last_used_at} />
        </TableCellData>
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
          <span style={{ color: theme.palette.text.secondary }}>
            {Language.upToDateLabel}
          </span>
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
      backgroundColor: theme.palette.action.hover,
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: theme.palette.text.secondary,
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
  templateIconWrapper: {
    // Same size then the avatar component
    width: 36,
    height: 36,
    padding: 2,

    "& img": {
      width: "100%",
    },
  },
}))
