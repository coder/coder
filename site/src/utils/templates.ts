export const formatTemplateActiveDevelopers = (num?: number): string => {
	if (num === undefined || num < 0) {
		// Loading
		return "-";
	}
	return num.toString();
};

export const formatTemplateBuildTime = (
	buildTimeMs?: number | null,
): string => {
	return buildTimeMs === undefined || buildTimeMs === null
		? "Unknown"
		: `${Math.round(buildTimeMs / 1000)}s`;
};
