import React from "react"

export interface ErrorSummaryProps {
  error: Error
}

export const ErrorSummary: React.FC<ErrorSummaryProps> = ({ error }) => {
  // TODO: More interesting error page
  return <div>{error.toString()}</div>
}
