export type TimeRange = {
	startedAfter: Date;
	startedBefore: Date;
};

const pad = (value: number): string => String(value).padStart(2, "0");

/**
 * Formats a Date as a browser-local "YYYY-MM-DDTHH:mm" string, the format
 * that <input type="datetime-local"> consumes and produces.
 */
export const toLocalInputValue = (date: Date): string => {
	const datePart = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
	const timePart = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
	return `${datePart}T${timePart}`;
};

/**
 * Parses a "YYYY-MM-DDTHH:mm" value from <input type="datetime-local"> as a
 * browser-local time. Returns null for empty or malformed values.
 */
export const fromLocalInputValue = (value: string): Date | null => {
	if (value === "") {
		return null;
	}
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return null;
	}
	return date;
};

/** Serializes a Date as RFC 3339 in UTC with second precision. */
export const toRFC3339 = (date: Date): string => {
	return date.toISOString().replace(/\.\d{3}Z$/, "Z");
};

/** The default sessions window: the 24 hours ending at now. */
export const defaultTimeRange = (now: Date): TimeRange => ({
	startedAfter: new Date(now.getTime() - 24 * 60 * 60 * 1000),
	startedBefore: now,
});

const TIME_RANGE_KEY_PATTERN = /started_(after|before):/;

/**
 * Appends the default time range to a filter query unless the query already
 * sets an explicit started_after or started_before. The default is kept in
 * memory instead of the URL so shared links resolve relative to the
 * viewer's current time.
 */
export const withDefaultTimeRange = (
	query: string,
	range: TimeRange,
): string => {
	if (TIME_RANGE_KEY_PATTERN.test(query)) {
		return query;
	}
	const suffix = [
		`started_after:"${toRFC3339(range.startedAfter)}"`,
		`started_before:"${toRFC3339(range.startedBefore)}"`,
	].join(" ");
	return query === "" ? suffix : `${query} ${suffix}`;
};

/**
 * Builds a filter query string that replaces any existing time range in
 * values with the given range while preserving all other filters. The
 * timestamps are quoted because RFC 3339 values contain colons that the
 * query parser would otherwise treat as key/value separators.
 */
export const setTimeRangeInQuery = (
	values: Record<string, string | undefined>,
	range: TimeRange,
): string => {
	const parts: string[] = [];
	for (const [key, value] of Object.entries(values)) {
		if (!value || key === "started_after" || key === "started_before") {
			continue;
		}
		parts.push(value.includes(" ") ? `${key}:"${value}"` : `${key}:${value}`);
	}
	parts.push(`started_after:"${toRFC3339(range.startedAfter)}"`);
	parts.push(`started_before:"${toRFC3339(range.startedBefore)}"`);
	return parts.join(" ");
};

/** Extracts an explicit time range from filter values, or null if absent. */
export const parseTimeRange = (
	values: Record<string, string | undefined>,
): TimeRange | null => {
	const after = values.started_after;
	const before = values.started_before;
	if (!after || !before) {
		return null;
	}
	const startedAfter = new Date(after);
	const startedBefore = new Date(before);
	if (
		Number.isNaN(startedAfter.getTime()) ||
		Number.isNaN(startedBefore.getTime())
	) {
		return null;
	}
	return { startedAfter, startedBefore };
};
