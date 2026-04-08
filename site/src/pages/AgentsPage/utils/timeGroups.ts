/**
 * Time-based grouping utility used by the sidebar to categorize
 * chats into "Today", "Yesterday", "This Week", and "Older".
 */
export const TIME_GROUPS = [
	"Today",
	"Yesterday",
	"This Week",
	"Older",
] as const;
type TimeGroup = (typeof TIME_GROUPS)[number];

export function getTimeGroup(dateStr: string): TimeGroup {
	const now = new Date();
	const date = new Date(dateStr);
	const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
	const yesterday = new Date(today);
	yesterday.setDate(yesterday.getDate() - 1);
	const weekAgo = new Date(today);
	weekAgo.setDate(weekAgo.getDate() - 7);

	if (date >= today) return "Today";
	if (date >= yesterday) return "Yesterday";
	if (date >= weekAgo) return "This Week";
	return "Older";
}
