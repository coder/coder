import Checkbox from "@material-ui/core/Checkbox"
import FormControl from "@material-ui/core/FormControl"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormGroup from "@material-ui/core/FormGroup"
import FormHelperText from "@material-ui/core/FormHelperText"
import FormLabel from "@material-ui/core/FormLabel"
import MenuItem from "@material-ui/core/MenuItem"
import makeStyles from "@material-ui/core/styles/makeStyles"
import Switch from "@material-ui/core/Switch"
import TextField from "@material-ui/core/TextField"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Section } from "components/Section/Section"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { FormikTouched, useFormik } from "formik"
import {
  defaultSchedule,
  emptySchedule,
} from "pages/WorkspaceSchedulePage/schedule"
import { ChangeEvent, FC } from "react"
import * as Yup from "yup"
import { getFormHelpers } from "../../util/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"
import { zones } from "./zones"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

export const Language = {
  errorNoDayOfWeek:
    "Must set at least one day of week if auto-start is enabled.",
  errorNoTime: "Start time is required when auto-start is enabled.",
  errorTime: "Time must be in HH:mm format (24 hours).",
  errorTimezone: "Invalid timezone.",
  errorNoStop:
    "Time until shutdown must be greater than zero when auto-stop is enabled.",
  errorTtlMax:
    "Please enter a limit that is less than or equal to 168 hours (7 days).",
  daysOfWeekLabel: "Days of Week",
  daySundayLabel: "Sunday",
  dayMondayLabel: "Monday",
  dayTuesdayLabel: "Tuesday",
  dayWednesdayLabel: "Wednesday",
  dayThursdayLabel: "Thursday",
  dayFridayLabel: "Friday",
  daySaturdayLabel: "Saturday",
  startTimeLabel: "Start time",
  startTimeHelperText: "Your workspace will automatically start at this time.",
  noStartTimeHelperText: "Your workspace will not automatically start.",
  timezoneLabel: "Timezone",
  ttlLabel: "Time until shutdown (hours)",
  ttlCausesShutdownHelperText: "Your workspace will shut down",
  ttlCausesShutdownAfterStart:
    "after its next start. We delay shutdown by this time whenever we detect activity",
  ttlCausesNoShutdownHelperText:
    "Your workspace will not automatically shut down.",
  formTitle: "Workspace schedule",
  startSection: "Start",
  startSwitch: "Auto-start",
  stopSection: "Stop",
  stopSwitch: "Auto-stop",
}

export interface WorkspaceScheduleFormProps {
  submitScheduleError?: Error | unknown
  initialValues: WorkspaceScheduleFormValues
  isLoading: boolean
  onCancel: () => void
  onSubmit: (values: WorkspaceScheduleFormValues) => void
  // for storybook
  initialTouched?: FormikTouched<WorkspaceScheduleFormValues>
  defaultTTL: number
}

export interface WorkspaceScheduleFormValues {
  autoStartEnabled: boolean
  sunday: boolean
  monday: boolean
  tuesday: boolean
  wednesday: boolean
  thursday: boolean
  friday: boolean
  saturday: boolean
  startTime: string
  timezone: string

  autoStopEnabled: boolean
  ttl: number
}

export const validationSchema = Yup.object({
  sunday: Yup.boolean(),
  monday: Yup.boolean().test(
    "at-least-one-day",
    Language.errorNoDayOfWeek,
    function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues

      if (!parent.autoStartEnabled) {
        return true
      } else {
        return ![
          parent.sunday,
          value,
          parent.tuesday,
          parent.wednesday,
          parent.thursday,
          parent.friday,
          parent.saturday,
        ].every((day) => day === false)
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
    .test("required-if-auto-start", Language.errorNoTime, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues
      if (parent.autoStartEnabled) {
        return value !== ""
      } else {
        return true
      }
    })
    .test("is-time-string", Language.errorTime, (value) => {
      if (value === "") {
        return true
      } else if (!/^[0-9][0-9]:[0-9][0-9]$/.test(value)) {
        return false
      } else {
        const parts = value.split(":")
        const HH = Number(parts[0])
        const mm = Number(parts[1])
        return HH >= 0 && HH <= 23 && mm >= 0 && mm <= 59
      }
    }),
  timezone: Yup.string()
    .ensure()
    .test("is-timezone", Language.errorTimezone, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues

      if (!parent.startTime) {
        return true
      } else {
        // Unfortunately, there's not a good API on dayjs at this time for
        // evaluating a timezone. Attempt to parse today in the supplied timezone
        // and return as valid if the function doesn't throw.
        try {
          dayjs.tz(dayjs(), value)
          return true
        } catch (e) {
          return false
        }
      }
    }),
  ttl: Yup.number()
    .integer()
    .min(0)
    .max(24 * 7 /* 7 days */, Language.errorTtlMax)
    .test("positive-if-auto-stop", Language.errorNoStop, function (value) {
      const parent = this.parent as WorkspaceScheduleFormValues
      if (parent.autoStopEnabled) {
        return Boolean(value)
      } else {
        return true
      }
    }),
})

export const WorkspaceScheduleForm: FC<
  React.PropsWithChildren<WorkspaceScheduleFormProps>
> = ({
  submitScheduleError,
  initialValues,
  isLoading,
  onCancel,
  onSubmit,
  initialTouched,
  defaultTTL,
}) => {
  const styles = useStyles()

  const form = useFormik<WorkspaceScheduleFormValues>({
    initialValues,
    onSubmit,
    validationSchema,
    initialTouched,
  })
  const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(
    form,
    submitScheduleError,
  )

  const checkboxes: Array<{ value: boolean; name: string; label: string }> = [
    {
      value: form.values.sunday,
      name: "sunday",
      label: Language.daySundayLabel,
    },
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
  ]

  const handleToggleAutoStart = async (e: ChangeEvent) => {
    form.handleChange(e)
    if (form.values.autoStartEnabled) {
      // disable autostart, clear values
      await form.setValues({
        ...form.values,
        autoStartEnabled: false,
        ...emptySchedule,
      })
    } else {
      // enable autostart, fill with defaults
      await form.setValues({
        ...form.values,
        autoStartEnabled: true,
        ...defaultSchedule(),
      })
    }
  }

  const handleToggleAutoStop = async (e: ChangeEvent) => {
    form.handleChange(e)
    if (form.values.autoStopEnabled) {
      // disable autostop, set TTL 0
      await form.setValues({ ...form.values, autoStopEnabled: false, ttl: 0 })
    } else {
      // enable autostop, fill with default TTL
      await form.setValues({
        ...form.values,
        autoStopEnabled: true,
        ttl: defaultTTL,
      })
    }
  }

  return (
    <FullPageForm title={Language.formTitle}>
      <form onSubmit={form.handleSubmit} className={styles.form}>
        <Stack>
          {Boolean(submitScheduleError) && (
            <AlertBanner severity="error" error={submitScheduleError} />
          )}
          <Section title={Language.startSection}>
            <FormControlLabel
              control={
                <Switch
                  name="autoStartEnabled"
                  checked={form.values.autoStartEnabled}
                  onChange={handleToggleAutoStart}
                  color="primary"
                />
              }
              label={Language.startSwitch}
            />
            <TextField
              {...formHelpers(
                "startTime",
                form.values.autoStartEnabled
                  ? Language.startTimeHelperText
                  : Language.noStartTimeHelperText,
              )}
              disabled={isLoading || !form.values.autoStartEnabled}
              InputLabelProps={{
                shrink: true,
              }}
              label={Language.startTimeLabel}
              type="time"
              fullWidth
            />

            <TextField
              {...formHelpers("timezone")}
              disabled={isLoading || !form.values.autoStartEnabled}
              InputLabelProps={{
                shrink: true,
              }}
              label={Language.timezoneLabel}
              select
              fullWidth
            >
              {zones.map((zone) => (
                <MenuItem key={zone} value={zone}>
                  {zone}
                </MenuItem>
              ))}
            </TextField>

            <FormControl
              component="fieldset"
              error={Boolean(form.errors.monday)}
            >
              <FormLabel className={styles.daysOfWeekLabel} component="legend">
                {Language.daysOfWeekLabel}
              </FormLabel>

              <FormGroup>
                {checkboxes.map((checkbox) => (
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={checkbox.value}
                        disabled={isLoading || !form.values.autoStartEnabled}
                        onChange={form.handleChange}
                        name={checkbox.name}
                        color="primary"
                        size="small"
                        disableRipple
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
          </Section>

          <Section title={Language.stopSection}>
            <FormControlLabel
              control={
                <Switch
                  name="autoStopEnabled"
                  checked={form.values.autoStopEnabled}
                  onChange={handleToggleAutoStop}
                  color="primary"
                />
              }
              label={Language.stopSwitch}
            />
            <TextField
              {...formHelpers("ttl", ttlShutdownAt(form.values.ttl), "ttl_ms")}
              disabled={isLoading || !form.values.autoStopEnabled}
              inputProps={{ min: 0, step: 1 }}
              label={Language.ttlLabel}
              type="number"
              fullWidth
            />
          </Section>

          <FormFooter onCancel={onCancel} isLoading={isLoading} />
        </Stack>
      </form>
    </FullPageForm>
  )
}

export const ttlShutdownAt = (formTTL: number): string => {
  if (formTTL < 1) {
    // Passing an empty value for TTL in the form results in a number that is not zero but less than 1.
    return Language.ttlCausesNoShutdownHelperText
  } else {
    return `${Language.ttlCausesShutdownHelperText} ${dayjs
      .duration(formTTL, "hours")
      .humanize()} ${Language.ttlCausesShutdownAfterStart}.`
  }
}

const useStyles = makeStyles({
  form: {
    "& input": {
      colorScheme: "dark",
    },
  },
  daysOfWeekLabel: {
    fontSize: 12,
  },
})
