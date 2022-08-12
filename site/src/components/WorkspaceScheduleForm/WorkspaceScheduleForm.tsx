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
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { Section } from "components/Section/Section"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { FormikTouched, useFormik } from "formik"
import { defaultSchedule } from "pages/WorkspaceSchedulePage/schedule"
import { defaultTTL } from "pages/WorkspaceSchedulePage/ttl"
import { ChangeEvent, FC } from "react"
import * as Yup from "yup"
import { getFormHelpersWithError } from "../../util/formUtils"
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
  errorNoDayOfWeek: "Must set at least one day of week if auto-start is enabled",
  errorNoTime: "Start time is required when auto-start is enabled",
  errorTime: "Time must be in HH:mm format (24 hours)",
  errorTimezone: "Invalid timezone",
  errorNoStop: "Time until shutdown must be greater than zero when auto-stop is enabled",
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
  timezoneLabel: "Timezone",
  ttlLabel: "Time until shutdown (hours)",
  ttlCausesShutdownHelperText: "Your workspace will shut down",
  ttlCausesShutdownAfterStart: "after start",
  ttlCausesNoShutdownHelperText: "Your workspace will not automatically shut down.",
  formTitle: "Workspace schedule",
  startSection: "Start",
  startSwitch: "Auto-start",
  stopSection: "Stop",
  stopSwitch: "Auto-stop",
}

export interface WorkspaceScheduleFormProps {
  submitScheduleError?: Error | unknown
  initialValues: WorkspaceScheduleFormValues
  maxTTLms?: number
  isLoading: boolean
  onCancel: () => void
  onSubmit: (values: WorkspaceScheduleFormValues) => void
  // for storybook
  initialTouched?: FormikTouched<WorkspaceScheduleFormValues>
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

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export const validationSchema = (maxTTLHours?: number) =>
  Yup.object({
    sunday: Yup.boolean(),
    monday: Yup.boolean().test("at-least-one-day", Language.errorNoDayOfWeek, function (value) {
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
    }),
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
      .max(maxTTLHours ?? 24 * 7 /* 1 week */)
      .test("positive-if-auto-stop", Language.errorNoStop, function (value) {
        const parent = this.parent as WorkspaceScheduleFormValues
        if (parent.autoStopEnabled) {
          return !!value
        } else {
          return true
        }
      }),
  })

export const WorkspaceScheduleForm: FC<WorkspaceScheduleFormProps> = ({
  submitScheduleError,
  initialValues,
  maxTTLms,
  isLoading,
  onCancel,
  onSubmit,
  initialTouched,
}) => {
  const styles = useStyles()

  const schema = validationSchema(maxTTLms ? Math.floor(maxTTLms / 3600000) : undefined)

  const form = useFormik<WorkspaceScheduleFormValues>({
    initialValues,
    onSubmit,
    validationSchema: schema,
    initialTouched,
  })
  const formHelpers = getFormHelpersWithError<WorkspaceScheduleFormValues>(
    form,
    submitScheduleError,
  )

  const checkboxes: Array<{ value: boolean; name: string; label: string }> = [
    { value: form.values.sunday, name: "sunday", label: Language.daySundayLabel },
    { value: form.values.monday, name: "monday", label: Language.dayMondayLabel },
    { value: form.values.tuesday, name: "tuesday", label: Language.dayTuesdayLabel },
    { value: form.values.wednesday, name: "wednesday", label: Language.dayWednesdayLabel },
    { value: form.values.thursday, name: "thursday", label: Language.dayThursdayLabel },
    { value: form.values.friday, name: "friday", label: Language.dayFridayLabel },
    { value: form.values.saturday, name: "saturday", label: Language.daySaturdayLabel },
  ]

  const handleToggleAutoStart = async (e: ChangeEvent) => {
    form.handleChange(e)
    // if enabling from empty values, fill with defaults
    if (!form.values.autoStartEnabled && !form.values.startTime) {
      await form.setValues({ ...form.values, autoStartEnabled: true, ...defaultSchedule() })
    }
  }

  const handleToggleAutoStop = async (e: ChangeEvent) => {
    form.handleChange(e)
    // if enabling from empty values, fill with defaults
    if (!form.values.autoStopEnabled && !form.values.ttl) {
      await form.setFieldValue("ttl", defaultTTL)
    }
  }

  return (
    <FullPageForm onCancel={onCancel} title={Language.formTitle}>
      <form onSubmit={form.handleSubmit} className={styles.form}>
        <Stack>
          {submitScheduleError && <ErrorSummary error={submitScheduleError} />}
          <Section title={Language.startSection}>
            <FormControlLabel
              control={
                <Switch
                  name="autoStartEnabled"
                  checked={form.values.autoStartEnabled}
                  onChange={handleToggleAutoStart}
                />
              }
              label={Language.startSwitch}
            />
            <TextField
              {...formHelpers("startTime", Language.startTimeHelperText)}
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

            <FormControl component="fieldset" error={Boolean(form.errors.monday)}>
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

              {form.errors.monday && <FormHelperText>{Language.errorNoDayOfWeek}</FormHelperText>}
            </FormControl>
          </Section>

          <Section title={Language.stopSection}>
            <FormControlLabel
              control={
                <Switch
                  name="autoStopEnabled"
                  checked={form.values.autoStopEnabled}
                  onChange={handleToggleAutoStop}
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
