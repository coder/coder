import { useState, FC, ReactElement } from "react"
import Collapse from "@mui/material/Collapse"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@mui/styles"
import { colors } from "theme/colors"
import ReportProblemOutlinedIcon from "@mui/icons-material/ReportProblemOutlined"
import Button from "@mui/material/Button"
import { useTranslation } from "react-i18next"

export interface WarningAlertProps {
  text: string
  dismissible?: boolean
  actions?: ReactElement[]
}

export const WarningAlert: FC<WarningAlertProps> = ({
  text,
  dismissible = false,
  actions = [],
}) => {
  const { t } = useTranslation("common")
  const [open, setOpen] = useState(true)
  const classes = useStyles()

  return (
    <Collapse in={open}>
      <Stack
        className={classes.alertContainer}
        direction="row"
        alignItems="center"
        spacing={0}
        justifyContent="space-between"
      >
        <Stack direction="row" spacing={1}>
          <ReportProblemOutlinedIcon
            fontSize="small"
            className={classes.alertIcon}
          />
          {text}
        </Stack>
        <Stack direction="row">
          {actions.length > 0 &&
            actions.map((action) => <div key={String(action)}>{action}</div>)}
          {dismissible && (
            <Button size="small" onClick={() => setOpen(false)}>
              {t("ctas.dismissCta")}
            </Button>
          )}
        </Stack>
      </Stack>
    </Collapse>
  )
}

const useStyles = makeStyles((theme) => ({
  alertContainer: {
    border: `1px solid ${colors.orange[7]}`,
    borderRadius: theme.shape.borderRadius,
    padding: `${theme.spacing(1)} ${theme.spacing(2)}`,
    backgroundColor: `${colors.gray[16]}`,
  },
  alertIcon: {
    color: `${colors.orange[7]}`,
  },
}))
