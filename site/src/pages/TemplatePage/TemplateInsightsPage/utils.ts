import { addDays, addHours, format, startOfDay, startOfHour } from "date-fns";

export function getDateRangeFilter({
  startDate,
  endDate,
  now,
  isToday,
}: {
  startDate: Date;
  endDate: Date;
  now: Date;
  isToday: (date: Date) => boolean;
}) {
  return {
    start_time: toISOLocal(startOfDay(startDate)),
    end_time: toISOLocal(
      isToday(endDate)
        ? startOfHour(addHours(now, 1))
        : startOfDay(addDays(endDate, 1)),
    ),
  };
}

function toISOLocal(d: Date) {
  return format(d, "yyyy-MM-dd'T'HH:mm:ssxxx");
}
