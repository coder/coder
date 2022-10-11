import Button from "@material-ui/core/Button"
import TextField from "@material-ui/core/TextField"
import { useState } from "react"
import { useTranslation } from "react-i18next"

interface EditHoursProps {
  handleSubmit: (hours: number) => void
}

export const EditHours = ({ handleSubmit }: EditHoursProps): JSX.Element => {
  const { t } = useTranslation("workspacePage")
  const [hours, setHours] = useState(0)
  return (
    <form onSubmit={() => handleSubmit(hours)}>
      <TextField
        inputProps={{ min: 0, step: 1 }}
        label={t("workspaceScheduleButton.hours")}
        value={hours}
        onChange={(e) => setHours(parseInt(e.target.value))}
        type="number"
      />
      <Button type="submit">
        {t("workspaceScheduleButton.submitDeadline")}
      </Button>
    </form>
  )
}
