import React from "react"

const Language = {
  unknownErrorMessage: "Unknown error",
}

export interface ErrorSummaryProps {
  error: Error | unknown
}

export const ErrorSummary: React.FC<ErrorSummaryProps> = ({ error }) => {
  // TODO: More interesting error page

  if (!(error instanceof Error)) {
    return <div>{Language.unknownErrorMessage}</div>
  } else {
    return <div>{error.toString()}</div>
  }
}
