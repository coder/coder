const customisedDateLocale: Intl.DateTimeFormatOptions = {
	second: "2-digit",
	minute: "2-digit",
	hour: "2-digit",
	day: "numeric",
	// Show the month as a short name
	month: "short",
	year: "numeric",
};

export function formatDate(
	date: Date,
	options: Intl.DateTimeFormatOptions = customisedDateLocale,
) {
	return date.toLocaleDateString(undefined, options);
}
