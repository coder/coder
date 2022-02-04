import React from "react"

export interface ErrorSummaryProps {
  error: Error | undefined
}

export const ErrorSummary: React.FC<ErrorSummaryProps> = ({ error }) => {
  // TODO: More interesting error page

  if (typeof error === "undefined") {
    return <div>{"Unknown error"}</div>
  }

  return <div>{error.toString()}</div>
}
