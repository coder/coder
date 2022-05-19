import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { CodeBlock } from "../CodeBlock/CodeBlock"
import { createCtas } from "./ReportButtons"

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

export const stackTraceAvailable = (stackTrace: string[]): StackTraceAvailableMsg => {
  return {
    type: "stackTraceAvailable",
    stackTrace,
  }
}

const setStackTrace = (model: ReportState, mappedStack: string[]): ReportState => {
  return {
    ...model,
    mappedStack,
  }
}

export const reducer = (model: ReportState, msg: ReportMessage): ReportState => {
  switch (msg.type) {
    case "stackTraceAvailable":
      return setStackTrace(model, msg.stackTrace)
    case "stackTraceUnavailable":
      return setStackTrace(model, ["Unable to get stack trace"])
  }
}

/**
 * A code block component that contains the error stack resulting from an error boundary trigger
 */
export const RuntimeErrorReport = ({ error, mappedStack }: ReportState): React.ReactElement => {
  const styles = useStyles()

  if (!mappedStack) {
    return <CodeBlock lines={[Language.reportLoading]} className={styles.codeBlock} />
  }

  const codeBlock = [
    "======================= STACK TRACE ========================",
    "",
    error.message,
    ...mappedStack,
    "",
    "============================================================",
  ]

  return <CodeBlock lines={codeBlock} className={styles.codeBlock} ctas={createCtas(codeBlock)} />
}

const useStyles = makeStyles(() => ({
  codeBlock: {
    minHeight: "auto",
    userSelect: "all",
    width: "100%",
  },
}))
