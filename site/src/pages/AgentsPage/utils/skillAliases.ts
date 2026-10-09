/** Skill sources in chatd's merge order, named by their alias prefix. */
export type SkillSource = "personal" | "org" | "workspace" | "plugin";

export type SkillSourceList<T extends { name: string }> = {
	source: SkillSource;
	/** Undefined while the source's list is still unknown. */
	skills: readonly T[] | undefined;
};

type SkillTriggerInput = {
	name: string;
	description: string;
	// Part of the skill's identity alongside its name.
	pluginName?: string;
};

export type SkillTrigger = SkillTriggerInput & {
	source: SkillSource;
	triggerText: string;
	// The qualified alias stays searchable even when the displayed trigger
	// is bare, so a typed qualified query keeps matching after collision
	// state changes mid-trigger.
	altTriggerText: string;
};

/**
 * Builds every "/" trigger. Workspace and plugin skills, and names in more
 * than one source (chatd's MergeSkills), are qualified, as is every trigger
 * while any list is unknown.
 */
export const resolveSkillTriggers = (
	lists: readonly SkillSourceList<SkillTriggerInput>[],
): SkillTrigger[] => {
	const hasUnknownList = lists.some((list) => list.skills === undefined);
	const sourceCountByName = new Map<string, number>();
	for (const list of lists) {
		for (const name of new Set(list.skills?.map((skill) => skill.name))) {
			sourceCountByName.set(name, (sourceCountByName.get(name) ?? 0) + 1);
		}
	}
	return lists.flatMap(({ source, skills }) =>
		(skills ?? []).map((skill) => {
			const qualified =
				source === "plugin"
					? `/plugin/${skill.pluginName}/${skill.name}`
					: `/${source}/${skill.name}`;
			const isQualified =
				source === "workspace" ||
				source === "plugin" ||
				hasUnknownList ||
				(sourceCountByName.get(skill.name) ?? 0) > 1;
			return {
				source,
				name: skill.name,
				description: skill.description,
				pluginName: skill.pluginName,
				triggerText: isQualified ? qualified : `/${skill.name}`,
				altTriggerText: qualified,
			};
		}),
	);
};
