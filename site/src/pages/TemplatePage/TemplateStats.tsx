import type { FC } from "react";
import { Link } from "react-router";
import type { Template, TemplateVersion } from "#/api/typesGenerated";
import { Stats, StatsItem } from "#/components/Stats/Stats";
import { createDayString } from "#/utils/createDayString";
import {
	formatTemplateActiveDevelopers,
	formatTemplateBuildTime,
} from "#/utils/templates";

interface TemplateStatsProps {
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
				label="Used by"
				value={
					<>
						{formatTemplateActiveDevelopers(template.active_user_count)}{" "}
						{template.active_user_count === 1 ? "developer" : "developers"}
					</>
				}
			/>
			<StatsItem
				label="Build time"
				value={formatTemplateBuildTime(template.build_time_stats.start.P50)}
			/>
			<StatsItem
				label="Active version"
				value={
					<Link to={`versions/${activeVersion.name}`}>
						{activeVersion.name}
					</Link>
				}
			/>
			<StatsItem
				label="Last updated"
				value={createDayString(template.updated_at)}
			/>
			<StatsItem label="Created by" value={template.created_by_name} />
		</Stats>
	);
};
