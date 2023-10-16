import { startOfDay, subDays } from "date-fns";

export const lastWeeks = (numberOfWeeks: number) => {
  const now = new Date();
  const endDate = startOfDay(subDays(now, 1));
  const startDate = startOfDay(subDays(endDate, 7 * numberOfWeeks));
  return { startDate, endDate };
};
