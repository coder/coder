import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { useState } from "react"
import { useTranslation } from "react-i18next"

interface EditHoursProps {
  handleSubmit: (hours: number) => void
  max: number
}

export const EditHours = ({
  handleSubmit,
  max,
}: EditHoursProps): JSX.Element => {
  const { t } = useTranslation("workspacePage")
  const [hours, setHours] = useState(1)
  const styles = useStyles()

  return (
    // hours is NaN when user deletes the value, so treat it as 0
    <form onSubmit={() => handleSubmit(Number.isNaN(hours) ? 0 : hours)}>
      <Stack direction="row" alignItems="baseline" spacing={1}>
        <TextField
          className={styles.inputField}
          inputProps={{ min: 0, max, step: 1 }}
          label={t("workspaceScheduleButton.hours")}
          value={hours.toString()}
          onChange={(e) => setHours(parseInt(e.target.value))}
          type="number"
        />
        <Button className={styles.button} type="submit" color="primary">
          {t("workspaceScheduleButton.submitDeadline")}
        </Button>
      </Stack>
    </form>
  )
}

const useStyles = makeStyles(() => ({
  inputField: {
    width: "70px",
    "& .MuiOutlinedInput-root": {
      height: "30px",
    },
  },
  button: {
    "&.MuiButton-root": {
      minHeight: "30px",
      height: "30px",
    },
  },
}))
