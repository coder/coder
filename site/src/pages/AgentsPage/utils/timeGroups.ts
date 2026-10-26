/**
 * Time-based grouping into "Today", "Yesterday", "Past 7 days", and
 * "Older".
 *
 * "Past 7 days" is a rolling window (today minus 7 days), not the
 * current calendar week, so the label stays accurate on Mondays
 * when calendar-week grouping would misfile items from last week.
 */
export const TIME_GROUPS = [
	"Today",
	"Yesterday",
	"Past 7 days",
	"Older",
] as const;
type TimeGroup = (typeof TIME_GROUPS)[number];

/** Returns the group of a date, relative to the day of `now`. */
export function getTimeGroup(
	dateStr: string,
	now: Date = new Date(),
): TimeGroup {
	const date = new Date(dateStr);
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
	const yesterday = new Date(today);
	yesterday.setDate(yesterday.getDate() - 1);
	const weekAgo = new Date(today);
	weekAgo.setDate(weekAgo.getDate() - 7);

	if (date >= today) return "Today";
	if (date >= yesterday) return "Yesterday";
	if (date >= weekAgo) return "Past 7 days";
	return "Older";
}
