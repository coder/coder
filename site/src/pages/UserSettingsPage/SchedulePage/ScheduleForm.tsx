import TextField from "@mui/material/TextField";
import { FormikContextType, useFormik } from "formik";
import { FC, useEffect, useState } from "react";
import * as Yup from "yup";
import { getFormHelpers } from "utils/formUtils";
import { LoadingButton } from "components/LoadingButton/LoadingButton";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";
import {
  UpdateUserQuietHoursScheduleRequest,
  UserQuietHoursScheduleResponse,
} from "api/typesGenerated";
import cronParser from "cron-parser";
import MenuItem from "@mui/material/MenuItem";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import { timeZones } from "utils/timeZones";
import { Alert } from "components/Alert/Alert";
import { useQueryClient } from "@tanstack/react-query";

dayjs.extend(utc);
import advancedFormat from "dayjs/plugin/advancedFormat";
import duration from "dayjs/plugin/duration";
import { userQuietHoursScheduleKey } from "api/queries/settings";
dayjs.extend(advancedFormat);
dayjs.extend(duration);
dayjs.extend(timezone);
dayjs.extend(relativeTime);

export interface ScheduleFormValues {
  startTime: string;
  timezone: string;
}

const validationSchema = Yup.object({
  startTime: Yup.string()
    .ensure()
    .test("is-time-string", "Time must be in HH:mm format.", (value) => {
      if (value === "") {
        return true;
      } else if (!/^[0-9][0-9]:[0-9][0-9]$/.test(value)) {
        return false;
      } else {
        const parts = value.split(":");
        const HH = Number(parts[0]);
        const mm = Number(parts[1]);
        return HH >= 0 && HH <= 23 && mm >= 0 && mm <= 59;
      }
    }),
  timezone: Yup.string().required(),
});

export interface ScheduleFormProps {
  isLoading: boolean;
  initialValues: UserQuietHoursScheduleResponse;
  refetch: () => Promise<void>;
  mutationError: unknown;
  onSubmit: (data: UpdateUserQuietHoursScheduleRequest) => void;
  // now can be set to force the time used for "Next occurrence" in tests.
  now?: Date;
}

export const ScheduleForm: FC<React.PropsWithChildren<ScheduleFormProps>> = ({
  isLoading,
  initialValues,
  refetch,
  mutationError,
  onSubmit,
  now,
}) => {
  // Force a re-render every 15 seconds to update the "Next occurrence" field.
  // The app re-renders by itself occasionally but this is just to be sure it
  // doesn't get stale.
  const [_, setTime] = useState<number>(Date.now());
  useEffect(() => {
    const interval = setInterval(() => setTime(Date.now()), 15000);
    return () => {
      clearInterval(interval);
    };
  }, []);

  const preferredTimezone = getPreferredTimezone();

  // If the user has a custom schedule, use that as the initial values.
  // Otherwise, use midnight in their preferred timezone.
  const formInitialValues = {
    startTime: "00:00",
    timezone: preferredTimezone,
  };
  if (initialValues.user_set) {
    formInitialValues.startTime = initialValues.time;
    formInitialValues.timezone = initialValues.timezone;
  }

  const form: FormikContextType<ScheduleFormValues> =
    useFormik<ScheduleFormValues>({
      initialValues: formInitialValues,
      validationSchema,
      onSubmit: async (values) => {
        onSubmit({
          schedule: timeToCron(values.startTime, values.timezone),
        });

        await refetch();
      },
    });
  const getFieldHelpers = getFormHelpers<ScheduleFormValues>(
    form,
    mutationError,
  );

  return (
    <Form onSubmit={form.handleSubmit}>
      <FormFields>
        {Boolean(mutationError) && <ErrorAlert error={mutationError} />}

        {!initialValues.user_set && (
          <Alert severity="info">
            You are currently using the default quiet hours schedule, which
            starts every day at <code>{initialValues.time}</code> in{" "}
            <code>{initialValues.timezone}</code>.
          </Alert>
        )}

        <Stack direction="row">
          <TextField
            {...getFieldHelpers("startTime")}
            disabled={isLoading}
            label="Start time"
            type="time"
            fullWidth
          />
          <TextField
            {...getFieldHelpers("timezone")}
            disabled={isLoading}
            label="Timezone"
            select
            fullWidth
          >
            {timeZones.map((zone) => (
              <MenuItem key={zone} value={zone}>
                {zone}
              </MenuItem>
            ))}
          </TextField>
        </Stack>

        <TextField
          disabled
          fullWidth
          label="Cron schedule"
          value={timeToCron(form.values.startTime, form.values.timezone)}
        />

        <TextField
          disabled
          fullWidth
          label="Next occurrence"
          value={formatNextRun(
            form.values.startTime,

            form.values.timezone,
            now,
          )}
        />

        <div>
          <LoadingButton
            loading={isLoading}
            disabled={isLoading}
            type="submit"
            variant="contained"
          >
            {!isLoading && "Update schedule"}
          </LoadingButton>
        </div>
      </FormFields>
    </Form>
  );
};

const getPreferredTimezone = () => {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
};

const timeToCron = (time: string, tz?: string) => {
  const [HH, mm] = time.split(":");
  let prefix = "";
  if (tz) {
    prefix = `CRON_TZ=${tz} `;
  }
  return `${prefix}${mm} ${HH} * * *`;
};

// evaluateNextRun returns a Date object of the next cron run time.
const evaluateNextRun = (
  time: string,
  tz: string,
  now: Date | undefined,
): Date => {
  // The cron-parser package doesn't accept a timezone in the cron string, but
  // accepts it as an option.
  const cron = timeToCron(time);
  const parsed = cronParser.parseExpression(cron, {
    currentDate: now,
    iterator: false,
    utc: false,
    tz,
  });

  return parsed.next().toDate();
};

const formatNextRun = (
  time: string,
  tz: string,
  now: Date | undefined,
): string => {
  const nowDjs = dayjs(now).tz(tz);
  const djs = dayjs(evaluateNextRun(time, tz, now)).tz(tz);
  let str = djs.format("h:mm A");
  if (djs.isSame(nowDjs, "day")) {
    str += " today";
  } else if (djs.isSame(nowDjs.add(1, "day"), "day")) {
    str += " tomorrow";
  } else {
    // This case will rarely ever be hit, as we're dealing with only times and
    // not dates, but it can be hit due to mismatched browser timezone to cron
    // timezone or due to daylight savings changes.
    str += ` on ${djs.format("dddd, MMMM D")}`;
  }

  str += ` (${djs.from(now)})`;

  return str;
};
