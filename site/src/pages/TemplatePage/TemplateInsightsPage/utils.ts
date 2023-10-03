import {
  addDays,
  addHours,
  format,
  startOfDay,
  startOfHour,
  isToday as isTodayDefault,
} from "date-fns";

type GetDateRangeFilterOptions = {
  startDate: Date;
  endDate: Date;
  // Testing purposes
  now?: Date;
  isToday?: (date: Date) => boolean;
};

export function getDateRangeFilter(props: GetDateRangeFilterOptions) {
  const {
    startDate,
    endDate,
    now = new Date(),
    isToday = isTodayDefault,
  } = props;

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
