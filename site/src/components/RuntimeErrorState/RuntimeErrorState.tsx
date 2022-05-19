import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import React, { useEffect, useReducer } from "react"
import { mapStackTrace } from "sourcemapped-stacktrace"
import { Margins } from "../Margins/Margins"
import { Section } from "../Section/Section"
import { reducer, RuntimeErrorReport, stackTraceAvailable, stackTraceUnavailable } from "./RuntimeErrorReport"

const Language = {
  title: "Coder encountered an error",
}

interface RuntimeErrorStateProps {
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
        <Section className={styles.reportContainer} title={<ErrorStateTitle />}>
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

  reportContainer: {
    display: "flex",
    justifyContent: "center",
    marginTop: theme.spacing(5),
  },
}))
