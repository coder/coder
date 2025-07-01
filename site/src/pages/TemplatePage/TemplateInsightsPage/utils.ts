import { startOfDay, subtractTime } from "utils/time";

export const lastWeeks = (numberOfWeeks: number) => {
	const now = new Date();
	const endDate = subtractTime(startOfDay(now), 1, "day");
	const startDate = subtractTime(endDate, 7 * numberOfWeeks, "day");
	return { startDate, endDate };
};
