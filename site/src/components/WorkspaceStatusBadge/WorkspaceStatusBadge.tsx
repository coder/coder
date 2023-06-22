import { Workspace } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import { FC, PropsWithChildren } from "react"
import { makeStyles } from "@mui/styles"
import { combineClasses } from "utils/combineClasses"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import {
  ImpendingDeletionBadge,
  ImpendingDeletionText,
} from "components/WorkspaceDeletion"
import { getDisplayWorkspaceStatus } from "utils/workspace"

export type WorkspaceStatusBadgeProps = {
  workspace: Workspace
  className?: string
}

export const WorkspaceStatusBadge: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const { text, icon, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
  )
  return (
    <ChooseOne>
      {/* <ImpendingDeletionBadge/> determines its own visibility */}
      <Cond condition={Boolean(ImpendingDeletionBadge({ workspace }))}>
        <ImpendingDeletionBadge workspace={workspace} />
      </Cond>
      <Cond>
        <Pill className={className} icon={icon} text={text} type={type} />
      </Cond>
    </ChooseOne>
  )
}

export const WorkspaceStatusText: FC<
  PropsWithChildren<WorkspaceStatusBadgeProps>
> = ({ workspace, className }) => {
  const styles = useStyles()
  const { text, type } = getDisplayWorkspaceStatus(
    workspace.latest_build.status,
  )

  return (
    <ChooseOne>
      {/* <ImpendingDeletionText/> determines its own visibility */}
      <Cond condition={Boolean(ImpendingDeletionText({ workspace }))}>
        <ImpendingDeletionText workspace={workspace} />
      </Cond>
      <Cond>
        <span
          role="status"
          data-testid="build-status"
          className={combineClasses([
            className,
            styles.root,
            styles[`type-${type}`],
          ])}
        >
          {text}
        </span>
      </Cond>
    </ChooseOne>
  )
}

const useStyles = makeStyles((theme) => ({
  root: { fontWeight: 600 },
  "type-error": {
    color: theme.palette.error.light,
  },
  "type-warning": {
    color: theme.palette.warning.light,
  },
  "type-success": {
    color: theme.palette.success.light,
  },
  "type-info": {
    color: theme.palette.info.light,
  },
  "type-undefined": {
    color: theme.palette.text.secondary,
  },
  "type-primary": {
    color: theme.palette.text.primary,
  },
  "type-secondary": {
    color: theme.palette.text.secondary,
  },
}))
