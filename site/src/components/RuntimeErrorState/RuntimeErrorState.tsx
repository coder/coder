import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import React, { useEffect, useReducer } from "react"
import { Link } from "react-router-dom"
import { mapStackTrace } from "sourcemapped-stacktrace"
import { Margins } from "../Margins/Margins"
import { Section } from "../Section/Section"
import { Typography } from "../Typography/Typography"
import { reducer, RuntimeErrorReport, stackTraceAvailable, stackTraceUnavailable } from "./RuntimeErrorReport"

const Language = {
  title: "Coder encountered an error",
  body: "Please copy the crash log using the button below and",
  link: "send it to us.",
}

export interface RuntimeErrorStateProps {
  error: Error
}

/**
 * A title for our error boundary UI
 */
const ErrorStateTitle = () => {
  const styles = useStyles()
  return (
    <Box className={styles.title} display="flex" alignItems="center">
      <ErrorOutlineIcon />
      <span>{Language.title}</span>
    </Box>
  )
}

/**
 * A description for our error boundary UI
 */
const ErrorStateDescription = () => {
  const styles = useStyles()
  return (
    <Typography variant="body2" color="textSecondary">
      {Language.body}&nbsp;
      <Link
        to="#"
        onClick={(e) => {
          window.location.href = "mailto:support@coder.com"
          e.preventDefault()
        }}
        className={styles.link}
      >
        {Language.link}
      </Link>
    </Typography>
  )
}

/**
 * An error UI that is displayed when our error boundary (ErrorBoundary.tsx) is triggered
 */
export const RuntimeErrorState: React.FC<RuntimeErrorStateProps> = ({ error }) => {
  const styles = useStyles()
  const [reportState, dispatch] = useReducer(reducer, { error, mappedStack: null })

  useEffect(() => {
    try {
      mapStackTrace(error.stack, (mappedStack) => dispatch(stackTraceAvailable(mappedStack)))
    } catch {
      dispatch(stackTraceUnavailable)
    }
  }, [error])

  return (
    <Box display="flex" flexDirection="column">
      <Margins>
        <Section className={styles.reportContainer} title={<ErrorStateTitle />} description={<ErrorStateDescription />}>
          <RuntimeErrorReport error={reportState.error} mappedStack={reportState.mappedStack} />
        </Section>
      </Margins>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    "& span": {
      paddingLeft: theme.spacing(1),
    },

    "& .MuiSvgIcon-root": {
      color: theme.palette.error.main,
    },
  },
  link: {
    textDecoration: "none",
    color: theme.palette.primary.main,
  },
  reportContainer: {
    display: "flex",
    justifyContent: "center",
    marginTop: theme.spacing(5),
  },
}))
