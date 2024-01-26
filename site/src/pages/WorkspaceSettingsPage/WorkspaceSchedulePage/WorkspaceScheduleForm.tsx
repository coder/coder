import Checkbox from "@mui/material/Checkbox";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import FormGroup from "@mui/material/FormGroup";
import FormHelperText from "@mui/material/FormHelperText";
import FormLabel from "@mui/material/FormLabel";
import MenuItem from "@mui/material/MenuItem";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import {
  HorizontalForm,
  FormFooter,
  FormSection,
  FormFields,
} from "components/Form/Form";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import advancedFormat from "dayjs/plugin/advancedFormat";
import duration from "dayjs/plugin/duration";
import relativeTime from "dayjs/plugin/relativeTime";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";
import { FormikTouched, useFormik } from "formik";
import {
  defaultSchedule,
  emptySchedule,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import { type ChangeEvent, type FC } from "react";
import * as Yup from "yup";
import { getFormHelpers } from "utils/formUtils";
import { timeZones } from "utils/timeZones";
import Tooltip from "@mui/material/Tooltip";
import { formatDuration, intervalToDuration } from "date-fns";
import { DisabledBadge } from "components/Badges/Badges";
import { TemplateAutostartRequirement } from "api/typesGenerated";

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc);
dayjs.extend(advancedFormat);
dayjs.extend(duration);
dayjs.extend(relativeTime);
dayjs.extend(timezone);

export const Language = {
  errorNoDayOfWeek:
    "Must set at least one day of week if autostart is enabled.",
  errorNoTime: "Start time is required when autostart is enabled.",
  errorTime: "Time must be in HH:mm format.",
  errorTimezone: "Invalid timezone.",
  errorNoStop:
    "Time until shutdown must be greater than zero when autostop is enabled.",
  errorTtlMax:
    "Please enter a limit that is less than or equal to 720 hours (30 days).",
  daysOfWeekLabel: "Days of Week",
  daySundayLabel: "Sun",
  dayMondayLabel: "Mon",
  dayTuesdayLabel: "Tue",
  dayWednesdayLabel: "Wed",
  dayThursdayLabel: "Thu",
  dayFridayLabel: "Fri",
  daySaturdayLabel: "Sat",
  startTimeLabel: "Start time",
  timezoneLabel: "Timezone",
  ttlLabel: "Time until shutdown (hours)",
  formTitle: "Workspace schedule",
  startSection: "Start",
  startSwitch: "Enable Autostart",
  stopSection: "Stop",
  stopSwitch: "Enable Autostop",
};

export interface WorkspaceScheduleFormProps {
  submitScheduleError?: unknown;
  initialValues: WorkspaceScheduleFormValues;
  isLoading: boolean;
  allowedTemplateAutoStartDays: TemplateAutostartRequirement["days_of_week"];
  allowTemplateAutoStop: boolean;
  allowTemplateAutoStart: boolean;
  onCancel: () => void;
  onSubmit: (values: WorkspaceScheduleFormValues) => void;
  // for storybook
  initialTouched?: FormikTouched<WorkspaceScheduleFormValues>;
  defaultTTL: number;
}

export interface WorkspaceScheduleFormValues {
  autostartEnabled: boolean;
  sunday: boolean;
  monday: boolean;
  tuesday: boolean;
  wednesday: boolean;
  thursday: boolean;
  friday: boolean;
  saturday: boolean;
  startTime: string;
  timezone: string;

  autostopEnabled: boolean;
  ttl: number;
}

export const validationSchema = Yup.object({
  sunday: Yup.boolean(),
  monday: Yup.boolean().test(
    "at-least-one-day",
    Language.errorNoDayOfWeek,
    function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues;

      if (!parent.autostartEnabled) {
        return true;
      } else {
        return ![
          parent.sunday,
          value,
          parent.tuesday,
          parent.wednesday,
          parent.thursday,
          parent.friday,
          parent.saturday,
        ].every((day) => day === false);
      }
    },
  ),
  tuesday: Yup.boolean(),
  wednesday: Yup.boolean(),
  thursday: Yup.boolean(),
  friday: Yup.boolean(),
  saturday: Yup.boolean(),

  startTime: Yup.string()
    .ensure()
    .test("required-if-autostart", Language.errorNoTime, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues;
      if (parent.autostartEnabled) {
        return value !== "";
      } else {
        return true;
      }
    })
    .test("is-time-string", Language.errorTime, (value) => {
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
  timezone: Yup.string()
    .ensure()
    .test("is-timezone", Language.errorTimezone, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues;

      if (!parent.startTime) {
        return true;
      } else {
        // Unfortunately, there's not a good API on dayjs at this time for
        // evaluating a timezone. Attempt to parse today in the supplied timezone
        // and return as valid if the function doesn't throw.
        try {
          dayjs.tz(dayjs(), value);
          return true;
        } catch (e) {
          return false;
        }
      }
    }),
  ttl: Yup.number()
    .min(0)
    .max(24 * 30 /* 30 days */, Language.errorTtlMax)
    .test("positive-if-autostop", Language.errorNoStop, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues;
      if (parent.autostopEnabled) {
        return Boolean(value);
      } else {
        return true;
      }
    }),
});

export const WorkspaceScheduleForm: FC<WorkspaceScheduleFormProps> = ({
  submitScheduleError,
  initialValues,
  isLoading,
  onCancel,
  onSubmit,
  initialTouched,
  defaultTTL,
  allowedTemplateAutoStartDays,
  allowTemplateAutoStop,
  allowTemplateAutoStart,
}) => {
  const form = useFormik<WorkspaceScheduleFormValues>({
    initialValues,
    onSubmit,
    validationSchema,
    initialTouched,
    enableReinitialize: true,
  });
  const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(
    form,
    submitScheduleError,
  );

  const checkboxes: Array<{ value: boolean; name: string; label: string }> = [
    {
      value: form.values.monday,
      name: "monday",
      label: Language.dayMondayLabel,
    },
    {
      value: form.values.tuesday,
      name: "tuesday",
      label: Language.dayTuesdayLabel,
    },
    {
      value: form.values.wednesday,
      name: "wednesday",
      label: Language.dayWednesdayLabel,
    },
    {
      value: form.values.thursday,
      name: "thursday",
      label: Language.dayThursdayLabel,
    },
    {
      value: form.values.friday,
      name: "friday",
      label: Language.dayFridayLabel,
    },
    {
      value: form.values.saturday,
      name: "saturday",
      label: Language.daySaturdayLabel,
    },
    {
      value: form.values.sunday,
      name: "sunday",
      label: Language.daySundayLabel,
    },
  ];

  const handleToggleAutostart = async (e: ChangeEvent) => {
    form.handleChange(e);
    if (form.values.autostartEnabled) {
      // disable autostart, clear values
      await form.setValues({
        ...form.values,
        autostartEnabled: false,
        ...emptySchedule,
      });
    } else {
      // enable autostart, fill with defaults
      await form.setValues({
        ...form.values,
        autostartEnabled: true,
        ...defaultSchedule(),
      });
    }
  };

  const handleToggleAutostop = async (e: ChangeEvent) => {
    form.handleChange(e);
    if (form.values.autostopEnabled) {
      // disable autostop, set TTL 0
      await form.setValues({ ...form.values, autostopEnabled: false, ttl: 0 });
    } else {
      // enable autostop, fill with default TTL
      await form.setValues({
        ...form.values,
        autostopEnabled: true,
        ttl: defaultTTL,
      });
    }
  };

  return (
    <HorizontalForm onSubmit={form.handleSubmit}>
      <FormSection
        title="Autostart"
        description={
          <>
            <div css={{ marginBottom: 16 }}>
              Select the time and days of week on which you want the workspace
              starting automatically.
            </div>
            {!allowTemplateAutoStart && (
              <Tooltip title="This option can be enabled in the template settings">
                <DisabledBadge />
              </Tooltip>
            )}
          </>
        }
      >
        <FormFields>
          <FormControlLabel
            control={
              <Switch
                disabled={!allowTemplateAutoStart}
                name="autostartEnabled"
                checked={form.values.autostartEnabled}
                onChange={handleToggleAutostart}
              />
            }
            label={Language.startSwitch}
          />
          <Stack direction="row">
            <TextField
              {...formHelpers("startTime")}
              // disabled if template does not allow autostart
              // or if primary feature is toggled off via the switch above
              disabled={
                isLoading ||
                !allowTemplateAutoStart ||
                !form.values.autostartEnabled
              }
              label={Language.startTimeLabel}
              type="time"
              fullWidth
            />
            <TextField
              {...formHelpers("timezone")}
              // disabled if template does not allow autostart
              // or if primary feature is toggled off via the switch above
              disabled={
                isLoading ||
                !allowTemplateAutoStart ||
                !form.values.autostartEnabled
              }
              label={Language.timezoneLabel}
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

          <FormControl component="fieldset" error={Boolean(form.errors.monday)}>
            <FormLabel css={{ fontSize: 12 }} component="legend">
              {Language.daysOfWeekLabel}
            </FormLabel>

            <FormGroup
              css={{
                display: "flex",
                flexDirection: "row",
                flexWrap: "wrap",
                paddingTop: 4,
              }}
            >
              {checkboxes.map((checkbox) => (
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={checkbox.value}
                      // template admins can disable the autostart feature in general,
                      // or they can disallow autostart on specific days of the week.
                      // also disabled if primary feature switch (above) is toggled off
                      disabled={
                        isLoading ||
                        !allowTemplateAutoStart ||
                        !allowedTemplateAutoStartDays.includes(checkbox.name) ||
                        !form.values.autostartEnabled
                      }
                      onChange={form.handleChange}
                      name={checkbox.name}
                      size="small"
                    />
                  }
                  key={checkbox.name}
                  label={checkbox.label}
                />
              ))}
            </FormGroup>

            {form.errors.monday && (
              <FormHelperText>{Language.errorNoDayOfWeek}</FormHelperText>
            )}
          </FormControl>
        </FormFields>
      </FormSection>

      <FormSection
        title="Autostop"
        description={
          <>
            <div css={{ marginBottom: 16 }}>
              Set how many hours should elapse after the workspace started
              before the workspace automatically shuts down. This will be
              extended by 1 hour after last activity in the workspace was
              detected.
            </div>
            {!allowTemplateAutoStop && (
              <Tooltip title="This option can be enabled in the template settings">
                <DisabledBadge />
              </Tooltip>
            )}
          </>
        }
      >
        <FormFields>
          <FormControlLabel
            control={
              <Switch
                name="autostopEnabled"
                checked={form.values.autostopEnabled}
                onChange={handleToggleAutostop}
                disabled={!allowTemplateAutoStop}
              />
            }
            label={Language.stopSwitch}
          />
          <TextField
            {...formHelpers("ttl", {
              helperText: ttlShutdownAt(form.values.ttl),
              backendFieldName: "ttl_ms",
            })}
            // disabled if autostop disabled at template level or
            // if autostop feature is toggled off via the switch above
            disabled={
              isLoading ||
              !allowTemplateAutoStop ||
              !form.values.autostopEnabled
            }
            inputProps={{ min: 0, step: "any" }}
            label={Language.ttlLabel}
            type="number"
            fullWidth
          />
        </FormFields>
      </FormSection>
      <FormFooter
        onCancel={onCancel}
        isLoading={isLoading}
        submitDisabled={
          (!allowTemplateAutoStart && !allowTemplateAutoStop) ||
          (!form.values.autostartEnabled && !form.values.autostopEnabled)
        }
      />
    </HorizontalForm>
  );
};

export const ttlShutdownAt = (formTTL: number): string => {
  if (formTTL === 0) {
    // Passing an empty value for TTL in the form results in a number that is not zero but less than 1.
    return "Your workspace will not automatically shut down.";
  } else {
    return `Your workspace will shut down ${formatDuration(
      intervalToDuration({ start: 0, end: formTTL * 60 * 60 * 1000 }),
      { delimiter: " and " },
    )} after its next start. We delay shutdown by this time whenever we detect activity.`;
  }
};
