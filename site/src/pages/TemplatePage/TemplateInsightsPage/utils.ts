import {
  addDays,
  addHours,
  format,
  startOfDay,
  startOfHour,
  isToday as isTodayDefault,
  startOfWeek,
  endOfDay,
  endOfWeek,
  isSunday,
  subWeeks,
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

export const lastWeeks = (numberOfWeeks: number) => {
  const now = new Date();
  const startDate = startOfWeek(subWeeks(now, numberOfWeeks));
  const endDate = isSunday(now) ? endOfDay(now) : endOfWeek(subWeeks(now, 1));
  return { startDate, endDate };
};
