import { useState, FC, Children } from "react"
import Collapse from "@material-ui/core/Collapse"
import { Stack } from "components/Stack/Stack"
import { makeStyles, Theme } from "@material-ui/core/styles"
import { colors } from "theme/colors"
import { useTranslation } from "react-i18next"
import { getErrorDetail, getErrorMessage } from "api/errors"
import { Expander } from "components/Expander/Expander"
import { Severity, AlertBannerProps } from "./alertTypes"
import { severityConstants } from "./severityConstants"
import { AlertBannerCtas } from "./AlertBannerCtas"

/**
 * @param children: the children to be displayed in the alert
 * @param severity: the level of alert severity (see ./severityTypes.ts)
 * @param text: default text to be displayed to the user; useful for warnings or as a fallback error message
 * @param error: should be passed in if the severity is 'Error'; warnings can use 'text' instead
 * @param actions: an array of CTAs passed in by the consumer
 * @param retry: a handler to retry the action that spawned the error
 * @param dismissible: determines whether or not the banner should have a `Dismiss` CTA
 * @param onDismiss: a handler that is called when the `Dismiss` CTA is clicked, after the animation has finished
 */
export const AlertBanner: FC<React.PropsWithChildren<AlertBannerProps>> = ({
  children,
  severity,
  text,
  error,
  actions = [],
  retry,
  dismissible = false,
  onDismiss,
}) => {
  const { t } = useTranslation("common")

  const [open, setOpen] = useState(true)

  // Set a fallback message if no text or children are provided.
  const defaultMessage =
    text ??
    (Children.count(children) === 0
      ? t("warningsAndErrors.somethingWentWrong")
      : "")

  // if an error is passed in, display that error, otherwise
  // display the text passed in, e.g. warning text
  const alertMessage = getErrorMessage(error, defaultMessage)

  // if we have an error, check if there's detail to display
  const detail = error ? getErrorDetail(error) : undefined
  const classes = useStyles({ severity, hasDetail: Boolean(detail) })

  const [showDetails, setShowDetails] = useState(false)

  return (
    <Collapse in={open} onExited={() => onDismiss && onDismiss()}>
      <Stack
        className={classes.alertContainer}
        direction="row"
        alignItems="center"
        spacing={0}
        justifyContent="space-between"
      >
        <Stack direction="row" alignItems="center" spacing={1}>
          {severityConstants[severity].icon}
          <Stack spacing={0}>
            {children}
            {alertMessage}
            {detail && (
              <Expander expanded={showDetails} setExpanded={setShowDetails}>
                <div>{detail}</div>
              </Expander>
            )}
          </Stack>
        </Stack>

        <AlertBannerCtas
          actions={actions}
          dismissible={dismissible}
          retry={retry}
          setOpen={setOpen}
        />
      </Stack>
    </Collapse>
  )
}

interface StyleProps {
  severity: Severity
  hasDetail: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  alertContainer: (props) => ({
    borderColor: severityConstants[props.severity].color,
    border: `1px solid ${colors.orange[7]}`,
    borderRadius: theme.shape.borderRadius,
    padding: `${theme.spacing(1)}px ${theme.spacing(2)}px`,
    backgroundColor: `${colors.gray[16]}`,
    textAlign: "left",

    "& span": {
      paddingTop: `${theme.spacing(0.25)}px`,
    },

    // targeting the alert icon rather than the expander icon
    "& svg:nth-child(2)": {
      marginTop: props.hasDetail ? `${theme.spacing(1)}px` : "inherit",
      marginRight: `${theme.spacing(1)}px`,
    },
  }),
}))
