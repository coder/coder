import { makeStyles } from "@material-ui/core/styles"
import { ReactElement } from "react"
import { CodeBlock } from "../CodeBlock/CodeBlock"
import { createCtas } from "./createCtas"

const Language = {
  reportLoading: "Generating crash report...",
}

interface ReportState {
  error: Error
  mappedStack: string[] | null
}

interface StackTraceAvailableMsg {
  type: "stackTraceAvailable"
  stackTrace: string[]
}

/**
 * stackTraceUnavailable is a Msg describing a stack trace not being available
 */
export const stackTraceUnavailable = {
  type: "stackTraceUnavailable",
} as const

type ReportMessage = StackTraceAvailableMsg | typeof stackTraceUnavailable

export const stackTraceAvailable = (
  stackTrace: string[],
): StackTraceAvailableMsg => {
  return {
    type: "stackTraceAvailable",
    stackTrace,
  }
}

const setStackTrace = (
  model: ReportState,
  mappedStack: string[],
): ReportState => {
  return {
    ...model,
    mappedStack,
  }
}

export const reducer = (
  model: ReportState,
  msg: ReportMessage,
): ReportState => {
  switch (msg.type) {
    case "stackTraceAvailable":
      return setStackTrace(model, msg.stackTrace)
    case "stackTraceUnavailable":
      return setStackTrace(model, ["Unable to get stack trace"])
  }
}

export const createFormattedStackTrace = (
  error: Error,
  mappedStack: string[] | null,
): string[] => {
  return [
    "======================= STACK TRACE ========================",
    "",
    error.message,
    ...(mappedStack ? mappedStack : []),
    "",
    "============================================================",
  ]
}

/**
 * A code block component that contains the error stack resulting from an error boundary trigger
 */
export const RuntimeErrorReport = ({
  error,
  mappedStack,
}: ReportState): ReactElement => {
  const styles = useStyles()

  if (!mappedStack) {
    return (
      <CodeBlock
        lines={[Language.reportLoading]}
        className={styles.codeBlock}
      />
    )
  }

  const formattedStackTrace = createFormattedStackTrace(error, mappedStack)
  return (
    <CodeBlock
      lines={formattedStackTrace}
      className={styles.codeBlock}
      ctas={createCtas(formattedStackTrace)}
    />
  )
}

const useStyles = makeStyles(() => ({
  codeBlock: {
    minHeight: "auto",
    userSelect: "all",
    width: "100%",
  },
}))
