import Checkbox from "@material-ui/core/Checkbox"
import FormControl from "@material-ui/core/FormControl"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormGroup from "@material-ui/core/FormGroup"
import FormHelperText from "@material-ui/core/FormHelperText"
import FormLabel from "@material-ui/core/FormLabel"
import makeStyles from "@material-ui/core/styles/makeStyles"
import TextField from "@material-ui/core/TextField"
import { useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { getFormHelpers } from "../../util/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"

export const Language = {
  errorNoDayOfWeek: "Must set at least one day of week",
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
  ttlLabel: "Runtime (minutes)",
  ttlHelperText: "Your workspace will automatically shutdown after the runtime.",
}

export interface WorkspaceScheduleFormProps {
  onCancel: () => void
  onSubmit: (values: WorkspaceScheduleFormValues) => Promise<void>
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

  startTime: Yup.string(),
  ttl: Yup.number().min(0).integer(),
})

export const WorkspaceScheduleForm: React.FC<WorkspaceScheduleFormProps> = ({ onCancel, onSubmit }) => {
  const styles = useStyles()

  const form = useFormik<WorkspaceScheduleFormValues>({
    initialValues: {
      sunday: false,
      monday: true,
      tuesday: true,
      wednesday: true,
      thursday: true,
      friday: true,
      saturday: false,

      startTime: "09:30",
      ttl: 120,
    },
    onSubmit,
    validationSchema,
  })
  const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(form)

  return (
    <FullPageForm onCancel={onCancel} title="Workspace Schedule">
      <form className={styles.form} onSubmit={form.handleSubmit}>
        <Stack className={styles.stack}>
          <TextField
            {...formHelpers("startTime", Language.startTimeHelperText)}
            InputLabelProps={{
              shrink: true,
            }}
            label={Language.startTimeLabel}
            type="time"
            variant="standard"
          />

          <FormControl component="fieldset" error={Boolean(form.errors.monday)}>
            <FormLabel className={styles.daysOfWeekLabel} component="legend">
              {Language.daysOfWeekLabel}
            </FormLabel>

            <FormGroup>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.sunday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="sunday"
                  />
                }
                label={Language.daySundayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.monday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="monday"
                  />
                }
                label={Language.dayMondayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.tuesday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="tuesday"
                  />
                }
                label={Language.dayTuesdayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.wednesday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="wednesday"
                  />
                }
                label={Language.dayWednesdayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.thursday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="thursday"
                  />
                }
                label={Language.dayThursdayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.friday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="friday"
                  />
                }
                label={Language.dayFridayLabel}
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={form.values.saturday}
                    disabled={!form.values.startTime}
                    onChange={form.handleChange}
                    name="saturday"
                  />
                }
                label={Language.daySaturdayLabel}
              />
            </FormGroup>
            {form.errors.monday && <FormHelperText>{Language.errorNoDayOfWeek}</FormHelperText>}
          </FormControl>

          <TextField
            {...formHelpers("ttl", Language.ttlHelperText)}
            inputProps={{ min: 0, step: 30 }}
            label={Language.ttlLabel}
            type="number"
            variant="standard"
          />

          <FormFooter onCancel={onCancel} isLoading={form.isSubmitting} />
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
