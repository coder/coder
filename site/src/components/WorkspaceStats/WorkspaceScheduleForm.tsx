import Checkbox from "@material-ui/core/Checkbox"
import FormControl from "@material-ui/core/FormControl"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormGroup from "@material-ui/core/FormGroup"
import FormHelperText from "@material-ui/core/FormHelperText"
import FormLabel from "@material-ui/core/FormLabel"
import Link from "@material-ui/core/Link"
import makeStyles from "@material-ui/core/styles/makeStyles"
import TextField from "@material-ui/core/TextField"
import dayjs from "dayjs"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { FieldErrors } from "../../api/errors"
import { getFormHelpers } from "../../util/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"

// REMARK: timezone plugin depends on UTC
//
// SEE: https://day.js.org/docs/en/timezone/timezone
dayjs.extend(utc)
dayjs.extend(timezone)

export const Language = {
  errorNoDayOfWeek: "Must set at least one day of week",
  errorTime: "Time must be in HH:mm format (24 hours)",
  errorTimezone: "Invalid timezone",
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
  ttlLabel: "TTL (hours)",
  ttlHelperText: "Your workspace will automatically shutdown after the TTL.",
}

export interface WorkspaceScheduleFormProps {
  fieldErrors?: FieldErrors
  initialValues?: WorkspaceScheduleFormValues
  isLoading: boolean
  onCancel: () => void
  onSubmit: (values: WorkspaceScheduleFormValues) => void
}

export interface WorkspaceScheduleFormValues {
  sunday: boolean
  monday: boolean
  tuesday: boolean
  wednesday: boolean
  thursday: boolean
  friday: boolean
  saturday: boolean

  startTime: string
  timezone: string
  ttl: number
}

export const validationSchema = Yup.object({
  sunday: Yup.boolean(),
  monday: Yup.boolean().test("at-least-one-day", Language.errorNoDayOfWeek, function (value) {
    const parent = this.parent as WorkspaceScheduleFormValues

    if (!parent.startTime) {
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
  ttl: Yup.number().min(0).integer(),
})

export const WorkspaceScheduleForm: React.FC<WorkspaceScheduleFormProps> = ({
  fieldErrors,
  initialValues = {
    sunday: false,
    monday: true,
    tuesday: true,
    wednesday: true,
    thursday: true,
    friday: true,
    saturday: false,

    startTime: "09:30",
    timezone: "",
    ttl: 5,
  },
  isLoading,
  onCancel,
  onSubmit,
}) => {
  const styles = useStyles()

  const form = useFormik<WorkspaceScheduleFormValues>({
    initialValues,
    onSubmit,
    validationSchema,
  })
  const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(form, fieldErrors)

  const checkboxes: Array<{ value: boolean; name: string; label: string }> = [
    { value: form.values.sunday, name: "sunday", label: Language.daySundayLabel },
    { value: form.values.monday, name: "monday", label: Language.dayMondayLabel },
    { value: form.values.tuesday, name: "tuesday", label: Language.dayTuesdayLabel },
    { value: form.values.wednesday, name: "wednesday", label: Language.dayWednesdayLabel },
    { value: form.values.thursday, name: "thursday", label: Language.dayThursdayLabel },
    { value: form.values.friday, name: "friday", label: Language.dayFridayLabel },
    { value: form.values.saturday, name: "saturday", label: Language.daySaturdayLabel },
  ]

  return (
    <FullPageForm onCancel={onCancel} title="Workspace Schedule">
      <form className={styles.form} onSubmit={form.handleSubmit}>
        <Stack className={styles.stack}>
          <TextField
            {...formHelpers("startTime", Language.startTimeHelperText)}
            disabled={form.isSubmitting || isLoading}
            InputLabelProps={{
              shrink: true,
            }}
            label={Language.startTimeLabel}
            type="time"
            variant="standard"
          />

          <TextField
            {...formHelpers(
              "timezone",
              <>
                Timezone must be a valid{" "}
                <Link href="https://en.wikipedia.org/wiki/List_of_tz_database_time_zones#List" target="_blank">
                  tz database name
                </Link>
              </>,
            )}
            disabled={form.isSubmitting || isLoading || !form.values.startTime}
            InputLabelProps={{
              shrink: true,
            }}
            label={Language.timezoneLabel}
            variant="standard"
          />

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
                      disabled={!form.values.startTime || form.isSubmitting || isLoading}
                      onChange={form.handleChange}
                      name={checkbox.name}
                    />
                  }
                  key={checkbox.name}
                  label={checkbox.label}
                />
              ))}
            </FormGroup>

            {form.errors.monday && <FormHelperText>{Language.errorNoDayOfWeek}</FormHelperText>}
          </FormControl>

          <TextField
            {...formHelpers("ttl", Language.ttlHelperText)}
            disabled={form.isSubmitting || isLoading}
            inputProps={{ min: 0, step: 1 }}
            label={Language.ttlLabel}
            type="number"
            variant="standard"
          />

          <FormFooter onCancel={onCancel} isLoading={form.isSubmitting || isLoading} />
        </Stack>
      </form>
    </FullPageForm>
  )
}

const useStyles = makeStyles({
  form: {
    display: "flex",
    justifyContent: "center",
  },
  stack: {
    // REMARK: 360 is 'arbitrary' in that it gives the helper text enough room
    //         to render on one line. If we change the text, we might want to
    //         adjust these. Without constraining the width, the date picker
    //         and number inputs aren't visually appealing or maximally usable.
    maxWidth: 360,
    minWidth: 360,
  },
  daysOfWeekLabel: {
    fontSize: 12,
  },
})
