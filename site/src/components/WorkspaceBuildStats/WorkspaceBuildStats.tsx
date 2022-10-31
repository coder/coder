import { Stats, StatsItem } from "components/Stats/Stats"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { displayWorkspaceBuildDuration } from "../../util/workspace"

export interface WorkspaceBuildStatsProps {
  build: WorkspaceBuild
}

export const WorkspaceBuildStats: FC<WorkspaceBuildStatsProps> = ({
  build,
}) => {
  const { t } = useTranslation("buildPage")

  return (
    <Stats>
      <StatsItem
        label={t("stats.workspace")}
        value={
          <Link to={`/@${build.workspace_owner_name}/${build.workspace_name}`}>
            {build.workspace_name}
          </Link>
        }
      />
      <StatsItem
        label={t("stats.duration")}
        value={displayWorkspaceBuildDuration(build)}
      />
      <StatsItem
        label={t("stats.startedAt")}
        value={new Date(build.created_at).toLocaleString()}
      />
      <StatsItem label={t("stats.action")} value={build.transition} />
    </Stats>
  )
}
