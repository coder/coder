import Box from "@material-ui/core/Box"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import React, { useEffect, useReducer } from "react"
import { mapStackTrace } from "sourcemapped-stacktrace"
import { Margins } from "../Margins/Margins"
import { Section } from "../Section/Section"
import { Typography } from "../Typography/Typography"
import {
  createFormattedStackTrace,
  reducer,
  RuntimeErrorReport,
  stackTraceAvailable,
  stackTraceUnavailable,
} from "./RuntimeErrorReport"

export const Language = {
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
const ErrorStateDescription = ({ emailBody }: { emailBody?: string }) => {
  const styles = useStyles()
  return (
    <Typography variant="body2" color="textSecondary">
      {Language.body}&nbsp;
      <Link
        href={`mailto:support@coder.com?subject=Error Report from Coder&body=${
          emailBody && emailBody.replace(/\r\n|\r|\n/g, "%0D%0A") // preserving line breaks
        }`}
        className={styles.link}
      >
        {Language.link}
      </Link>
    </Typography>
  );
}

/**
 * An error UI that is displayed when our error boundary (ErrorBoundary.tsx) is triggered
 */
export const RuntimeErrorState: React.FC<React.PropsWithChildren<RuntimeErrorStateProps>> = ({ error }) => {
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
        <Section
          className={styles.reportContainer}
          title={<ErrorStateTitle />}
          description={
            <ErrorStateDescription
              emailBody={createFormattedStackTrace(reportState.error, reportState.mappedStack).join(
                "\r\n",
              )}
            />
          }
        >
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
