import React from "react"

export interface ErrorProps {
  error: Error
}

export const Error: React.FC<ErrorProps> = ({ error }) => {
  // TODO: More interesting error page
  return <div>{error.toString()}</div>
}
