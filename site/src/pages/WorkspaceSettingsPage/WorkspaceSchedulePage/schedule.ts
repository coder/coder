import * as cronParser from "cron-parser";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import { extractTimezone, stripTimezone } from "utils/schedule";
import { Autostop } from "./ttl";
import { WorkspaceScheduleFormValues } from "./WorkspaceScheduleForm";
import map from "lodash/map";
import some from "lodash/some";

// REMARK: timezone plugin depends on UTC
//
// SEE: https://day.js.org/docs/en/timezone/timezone
dayjs.extend(utc);
dayjs.extend(timezone);

export interface AutostartSchedule {
  sunday: boolean;
  monday: boolean;
  tuesday: boolean;
  wednesday: boolean;
  thursday: boolean;
  friday: boolean;
  saturday: boolean;
  startTime: string;
  timezone: string;
}

export type Autostart = {
  autostartEnabled: boolean;
} & AutostartSchedule;

export const emptySchedule = {
  sunday: false,
  monday: false,
  tuesday: false,
  wednesday: false,
  thursday: false,
  friday: false,
  saturday: false,

  startTime: "",
  timezone: "",
};

export const defaultSchedule = (): AutostartSchedule => ({
  sunday: false,
  monday: true,
  tuesday: true,
  wednesday: true,
  thursday: true,
  friday: true,
  saturday: false,

  startTime: "09:30",
  timezone: dayjs.tz.guess(),
});

const transformSchedule = (schedule: string) => {
  const timezone = extractTimezone(schedule, dayjs.tz.guess());

  const expression = cronParser.parseExpression(stripTimezone(schedule));

  const HH = expression.fields.hour.join("").padStart(2, "0");
  const mm = expression.fields.minute.join("").padStart(2, "0");

  const weeklyFlags = [false, false, false, false, false, false, false];

  for (const day of expression.fields.dayOfWeek) {
    weeklyFlags[day % 7] = true;
  }

  return {
    sunday: weeklyFlags[0],
    monday: weeklyFlags[1],
    tuesday: weeklyFlags[2],
    wednesday: weeklyFlags[3],
    thursday: weeklyFlags[4],
    friday: weeklyFlags[5],
    saturday: weeklyFlags[6],
    startTime: `${HH}:${mm}`,
    timezone,
  };
};

export const scheduleToAutostart = (schedule?: string): Autostart => {
  if (schedule) {
    return {
      autostartEnabled: true,
      ...transformSchedule(schedule),
    };
  } else {
    return { autostartEnabled: false, ...emptySchedule };
  }
};

export const scheduleChanged = (
  initialValues: Autostart | Autostop,
  formValues: WorkspaceScheduleFormValues,
): boolean =>
  some(
    map(
      { ...initialValues },
      (v: boolean | string, k: keyof typeof initialValues) =>
        formValues[k] !== v,
    ),
  );
