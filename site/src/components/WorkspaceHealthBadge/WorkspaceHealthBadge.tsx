import { Workspace } from "api/typesGenerated"
import { Pill } from "components/Pill/Pill"
import { FC, PropsWithChildren } from "react"
import { ErrorIcon } from "components/Icons/ErrorIcon"
import FavoriteIcon from "@mui/icons-material/Favorite"
import { Maybe } from "components/Conditionals/Maybe"

export type WorkspaceHealthBadgeProps = {
  workspace: Workspace
  className?: string
}

export const WorkspaceHealthBadge: FC<
  PropsWithChildren<WorkspaceHealthBadgeProps>
> = ({ workspace, className }) => {
  return (
    <Maybe
      condition={["starting", "running", "stopping"].includes(
        workspace.latest_build.status,
      )}
    >
      <Pill
        className={className}
        icon={workspace.health.healthy ? <FavoriteIcon /> : <ErrorIcon />}
        text={workspace.health.healthy ? "Healthy" : "Unhealthy"}
        type={workspace.health.healthy ? "success" : "warning"}
      />
    </Maybe>
  )
}
