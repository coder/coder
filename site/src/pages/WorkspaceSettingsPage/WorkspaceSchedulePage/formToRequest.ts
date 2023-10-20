import * as TypesGen from "api/typesGenerated";
import { WorkspaceScheduleFormValues } from "./WorkspaceScheduleForm";

export const formValuesToAutostartRequest = (
  values: WorkspaceScheduleFormValues,
): TypesGen.UpdateWorkspaceAutostartRequest => {
  if (!values.autostartEnabled || !values.startTime) {
    return {
      schedule: "",
    };
  }

  const [HH, mm] = values.startTime.split(":");

  // Note: Space after CRON_TZ if timezone is defined
  const preparedTZ = values.timezone ? `CRON_TZ=${values.timezone} ` : "";

  const makeCronString = (dow: string) => `${preparedTZ}${mm} ${HH} * * ${dow}`;

  const days = [
    values.sunday,
    values.monday,
    values.tuesday,
    values.wednesday,
    values.thursday,
    values.friday,
    values.saturday,
  ];

  const isEveryDay = days.every((day) => day);

  const isMonThroughFri =
    !values.sunday &&
    values.monday &&
    values.tuesday &&
    values.wednesday &&
    values.thursday &&
    values.friday &&
    !values.saturday &&
    !values.sunday;

  // Handle special cases, falling through to comma-separation
  if (isEveryDay) {
    return {
      schedule: makeCronString("*"),
    };
  } else if (isMonThroughFri) {
    return {
      schedule: makeCronString("1-5"),
    };
  } else {
    const dow = days.reduce((previous, current, idx) => {
      if (!current) {
        return previous;
      } else {
        const prefix = previous ? "," : "";
        return previous + prefix + idx;
      }
    }, "");

    return {
      schedule: makeCronString(dow),
    };
  }
};

export const formValuesToTTLRequest = (
  values: WorkspaceScheduleFormValues,
): TypesGen.UpdateWorkspaceTTLRequest => {
  return {
    // minutes to nanoseconds
    ttl_ms:
      values.autostopEnabled && values.ttl
        ? values.ttl * 60 * 60 * 1000
        : undefined,
  };
};
