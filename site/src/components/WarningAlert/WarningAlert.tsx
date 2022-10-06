import { useState, FC, ReactElement } from "react"
import Collapse from "@material-ui/core/Collapse"
import { Stack } from "components/Stack/Stack"
import { makeStyles, Theme } from "@material-ui/core/styles"
import { colors } from "theme/colors"
import ReportProblemOutlinedIcon from "@material-ui/icons/ReportProblemOutlined"
import Button from "@material-ui/core/Button"
import { useTranslation } from "react-i18next"
import ErrorOutlineOutlinedIcon from "@material-ui/icons/ErrorOutlineOutlined"

type Severity = "warning" | "error"

export interface WarningAlertProps {
  text: string
  severity: Severity
  dismissible?: boolean
  actions?: ReactElement[]
}

const severityConstants: Record<Severity, { color: string; icon: ReactElement }> = {
  warning: {
    color: colors.orange[7],
    icon: <ReportProblemOutlinedIcon fontSize="small" style={{ color: colors.orange[7] }} />,
  },
  error: {
    color: colors.red[7],
    icon: <ErrorOutlineOutlinedIcon fontSize="small" style={{ color: colors.red[7] }} />,
  },
}

export const WarningAlert: FC<WarningAlertProps> = ({
  text,
  severity,
  dismissible = false,
  actions = [],
}) => {
  const { t } = useTranslation("common")
  const [open, setOpen] = useState(true)
  const classes = useStyles({ severity })

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
          {severityConstants[severity].icon}
          {text}
        </Stack>
        <Stack direction="row">
          {actions.length > 0 && actions.map((action) => <div key={String(action)}>{action}</div>)}
          {dismissible && (
            <Button size="small" onClick={() => setOpen(false)} variant="outlined">
              {t("ctas.dismissCta")}
            </Button>
          )}
        </Stack>
      </Stack>
    </Collapse>
  )
}

interface StyleProps {
  severity: Severity
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  alertContainer: (props) => ({
    borderColor: severityConstants[props.severity].color,
    border: `1px solid ${colors.orange[7]}`,
    borderRadius: theme.shape.borderRadius,
    padding: `${theme.spacing(1)}px ${theme.spacing(2)}px`,
    backgroundColor: `${colors.gray[16]}`,
  }),
}))
