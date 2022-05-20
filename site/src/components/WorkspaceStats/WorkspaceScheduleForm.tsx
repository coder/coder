import Checkbox from "@material-ui/core/Checkbox"
import FormControl from "@material-ui/core/FormControl"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormGroup from "@material-ui/core/FormGroup"
import FormLabel from "@material-ui/core/FormLabel"
import makeStyles from "@material-ui/core/styles/makeStyles"
import TextField from "@material-ui/core/TextField"
import { useFormik } from "formik"
import React from "react"
import { getFormHelpers } from "../../util/formUtils"
import { FormFooter } from "../FormFooter/FormFooter"
import { FullPageForm } from "../FullPageForm/FullPageForm"
import { Stack } from "../Stack/Stack"

export const Language = {
  daysOfWeekLabel: "Days of Week",
  daySundayLabel: "Sunday",
  dayMondayLabel: "Monday",
  dayTuesdayLabel: "Tuesday",
  dayWednesdayLabel: "Wednesday",
  dayThursdayLabel: "Thursday",
  dayFridayLabel: "Friday",
  daySaturdayLabel: "Saturday",
  startTimeLabel: "Start time",
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

      startTime: "",
      ttl: 0,
    },
    onSubmit,
  })
  const formHelpers = getFormHelpers<WorkspaceScheduleFormValues>(form)

  return (
    <FullPageForm onCancel={onCancel} title="Workspace Schedule">
      <form className={styles.form} onSubmit={form.handleSubmit}>
        <Stack className={styles.stack}>
          <TextField
            {...formHelpers("startTime")}
            InputLabelProps={{
              shrink: true,
            }}
            label={Language.startTimeLabel}
            type="time"
            variant="standard"
          />

          <FormControl component="fieldset">
            <FormLabel className={styles.daysOfWeekLabel} component="legend">
              {Language.daysOfWeekLabel}
            </FormLabel>

            <FormGroup>
              <FormControlLabel
                control={<Checkbox checked={form.values.sunday} onChange={form.handleChange} name="sunday" />}
                label={Language.daySundayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.monday} onChange={form.handleChange} name="monday" />}
                label={Language.dayMondayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.tuesday} onChange={form.handleChange} name="tuesday" />}
                label={Language.dayTuesdayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.wednesday} onChange={form.handleChange} name="wednesday" />}
                label={Language.dayWednesdayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.thursday} onChange={form.handleChange} name="thursday" />}
                label={Language.dayThursdayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.friday} onChange={form.handleChange} name="friday" />}
                label={Language.dayFridayLabel}
              />
              <FormControlLabel
                control={<Checkbox checked={form.values.saturday} onChange={form.handleChange} name="saturday" />}
                label={Language.daySaturdayLabel}
              />
            </FormGroup>
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
