/** Halloween easter eggs run Oct 24 to 31 in the viewer's local time. */
const isHalloween = (date: Date): boolean =>
	date.getMonth() === 9 && date.getDate() >= 24;

/**
 * Whether to show Halloween easter eggs. `?halloween=1` forces them on and
 * `?halloween=0` forces them off, so they can be reviewed on any day.
 */
export const isHalloweenEnabled = (
	date: Date = new Date(),
	search: string = window.location.search,
): boolean => {
	const override = new URLSearchParams(search).get("halloween");
	if (override === "1") {
		return true;
	}
	if (override === "0") {
		return false;
	}
	return isHalloween(date);
};
