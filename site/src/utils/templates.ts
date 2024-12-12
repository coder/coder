import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(duration);
dayjs.extend(relativeTime);

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
		: `${Math.round(dayjs.duration(buildTimeMs, "milliseconds").asSeconds())}s`;
};
