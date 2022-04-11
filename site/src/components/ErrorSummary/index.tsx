import React from "react"

export interface ErrorSummaryProps {
  error: Error | unknown
}

export const ErrorSummary: React.FC<ErrorSummaryProps> = ({ error }) => {
  // TODO: More interesting error page

  if (!(error instanceof Error)) {
    return <div>{"Unknown error"}</div>
  }

  return <div>{error.toString()}</div>
}
