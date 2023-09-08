/* eslint-disable eslint-comments/disable-enable-pair -- Solve below */
/* eslint-disable import/no-duplicates -- https://github.com/date-fns/date-fns/issues/1677 */
import formatRelative from "date-fns/formatRelative";
import subDays from "date-fns/subDays";

export const createDisplayDate = (
  date: Date,
  base: Date = new Date(),
): string => {
  const lastWeek = subDays(base, 7);
  if (date >= lastWeek) {
    return formatRelative(date, base).split(" at ")[0];
  }
  return date.toLocaleDateString();
};
