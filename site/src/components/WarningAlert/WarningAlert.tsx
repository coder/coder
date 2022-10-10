import { useState, FC, ReactElement } from "react"
import Collapse from "@material-ui/core/Collapse"
import { Stack } from "components/Stack/Stack"
import { makeStyles, Theme } from "@material-ui/core/styles"
import { colors } from "theme/colors"
import ReportProblemOutlinedIcon from "@material-ui/icons/ReportProblemOutlined"
import Button from "@material-ui/core/Button"
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
            <Button
              size="small"
              onClick={() => setOpen(false)}
              variant="outlined"
            >
              {t("ctas.dismissCta")}
            </Button>
          )}
        </Stack>
      </Stack>
    </Collapse>
  )
}

const useStyles = makeStyles<Theme>((theme) => ({
  alertContainer: {
    border: `1px solid ${colors.orange[7]}`,
    borderRadius: theme.shape.borderRadius,
    padding: `${theme.spacing(1)}px ${theme.spacing(2)}px`,
    backgroundColor: `${colors.gray[16]}`,
  },
  alertIcon: {
    color: `${colors.orange[7]}`,
  },
}))
