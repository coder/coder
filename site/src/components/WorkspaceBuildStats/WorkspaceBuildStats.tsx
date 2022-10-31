import { Stats, StatsItem } from "components/Stats/Stats"
import { FC } from "react"
import { Link } from "react-router-dom"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { displayWorkspaceBuildDuration } from "../../util/workspace"

export interface WorkspaceBuildStatsProps {
  build: WorkspaceBuild
}

export const WorkspaceBuildStats: FC<WorkspaceBuildStatsProps> = ({
  build,
}) => {
  return (
    <Stats>
      <StatsItem
        label="Workspace"
        value={
          <Link to={`/@${build.workspace_owner_name}/${build.workspace_name}`}>
            {build.workspace_name}
          </Link>
        }
      />
      <StatsItem
        label="Duration"
        value={displayWorkspaceBuildDuration(build)}
      />
      <StatsItem
        label="Started at"
        value={new Date(build.created_at).toLocaleString()}
      />
      <StatsItem label="Action" value={build.transition} />
    </Stats>
  )
}
