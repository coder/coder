/** Skill sources in chatd's merge order, named by their alias prefix. */
export type SkillSource = "personal" | "org" | "workspace";

export type SkillSourceList<T extends { name: string }> = {
	source: SkillSource;
	/** Undefined while the source's list is still unknown. */
	skills: readonly T[] | undefined;
};

export type SkillTrigger = {
	source: SkillSource;
	name: string;
	description: string;
	triggerText: string;
	// The qualified alias stays searchable even when the displayed trigger
	// is bare, so a typed qualified query keeps matching after collision
	// state changes mid-trigger.
	altTriggerText: string;
};

/**
 * Builds every "/" trigger. Names in more than one source are qualified in
 * each (chatd's MergeSkills); unknown lists and workspace skills also are.
 */
export const resolveSkillTriggers = (
	lists: readonly SkillSourceList<{ name: string; description: string }>[],
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
			const qualified = `/${source}/${skill.name}`;
			const isQualified =
				source === "workspace" ||
				hasUnknownList ||
				(sourceCountByName.get(skill.name) ?? 0) > 1;
			return {
				source,
				name: skill.name,
				description: skill.description,
				triggerText: isQualified ? qualified : `/${skill.name}`,
				altTriggerText: qualified,
			};
		}),
	);
};
