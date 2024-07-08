import type { FC } from "react";
import { Link } from "react-router-dom";
import type { Template, TemplateVersion } from "api/typesGenerated";
import { Stats, StatsItem } from "components/Stats/Stats";
import { createDayString } from "utils/createDayString";
import {
  formatTemplateBuildTime,
  formatTemplateActiveDevelopers,
} from "utils/templates";

const Language = {
  usedByLabel: "Used by",
  buildTimeLabel: "Build time",
  activeVersionLabel: "Active version",
  lastUpdateLabel: "Last updated",
  developerPlural: "developers",
  developerSingular: "developer",
  createdByLabel: "Created by",
};

export interface TemplateStatsProps {
  template: Template;
  activeVersion: TemplateVersion;
}

export const TemplateStats: FC<TemplateStatsProps> = ({
  template,
  activeVersion,
}) => {
  return (
    <Stats>
      <StatsItem
        label={Language.usedByLabel}
        value={
          <>
            {formatTemplateActiveDevelopers(template.active_user_count)}{" "}
            {template.active_user_count === 1
              ? Language.developerSingular
              : Language.developerPlural}
          </>
        }
      />
      <StatsItem
        label={Language.buildTimeLabel}
        value={formatTemplateBuildTime(template.build_time_stats.start.P50)}
      />
      <StatsItem
        label={Language.activeVersionLabel}
        value={
          <Link to={`versions/${activeVersion.name}`}>
            {activeVersion.name}
          </Link>
        }
      />
      <StatsItem
        label={Language.lastUpdateLabel}
        value={createDayString(template.updated_at)}
      />
      <StatsItem
        label={Language.createdByLabel}
        value={template.created_by_name}
      />
    </Stats>
  );
};
